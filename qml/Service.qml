import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  visible: false

  property var shell: null
  property var manifest: null
  property var snapshot: ({ version: 1, connected: false, projects: [], runtime: {}, routes: [], diagnostics: [] })
  property string lastError: ""
  property string actionStatus: ""
  property var logs: []
  property string logsTarget: ""
  property bool logsLoading: false
  property var composeImport: []
  property string composeImportPath: ""
  property double _clockMs: Date.now()

  readonly property string runtimeBase: Quickshell.env("XDG_RUNTIME_DIR") || ""
  readonly property string snapshotPath: runtimeBase !== "" ? runtimeBase + "/omastack/state.json" : ""
  readonly property bool connected: snapshot && snapshot.connected === true && snapshot.generatedAt && (_clockMs - Date.parse(snapshot.generatedAt)) < 90000
  readonly property var projects: snapshot && snapshot.projects ? snapshot.projects : []
  readonly property var runtime: snapshot && snapshot.runtime ? snapshot.runtime : ({})
  readonly property int runningCount: countState("running")
  readonly property int stoppedCount: countState("stopped")
  readonly property int unhealthyCount: countState("unhealthy")
  readonly property int crashedCount: countState("crashed")
  readonly property int transitioningCount: countState("starting") + countState("stopping")
  readonly property int totalCount: runningCount + stoppedCount + unhealthyCount + crashedCount + transitioningCount

  property var requestQueue: []
  property string _stdout: ""
  property string _stderr: ""
  property string _logsStdout: ""
  property string _logsStderr: ""
  property string _activeRequestMethod: ""
  property bool _activeRequestSilent: false

  function countState(state) {
    var count = 0
    var values = root.runtime || {}
    for (var id in values) if (values[id] && values[id].status === state) count++
    return count
  }

  function parseSnapshot(content) {
    try {
      var parsed = JSON.parse(String(content || ""))
      if (!parsed || typeof parsed !== "object" || parsed.version !== 1) throw new Error("unsupported snapshot")
      root.snapshot = parsed
      root.lastError = ""
    } catch (error) {
      root.lastError = "Could not read the OmaStack runtime snapshot"
    }
  }

  function refresh() {
    if (root.snapshotPath !== "") stateFile.reload()
  }

  function validTarget(target) {
    var value = String(target || "")
    return value.length > 0 && value.length <= 256 && value.indexOf("\u0000") === -1
  }

  function request(method, params, silent) {
    if (!/^[a-z][a-z0-9.-]{0,63}$/.test(String(method || ""))) {
      root.lastError = "Invalid OmaStack request"
      return false
    }
    var next = root.requestQueue.slice()
    next.push({ method: String(method), params: params || {}, silent: silent === true })
    root.requestQueue = next
    pumpRequests()
    return true
  }

  function lifecycle(method, target) {
    if (!validTarget(target)) { root.lastError = "Choose a valid project or service"; return false }
    return request(method, { target: String(target) })
  }

  function start(target) { return lifecycle("start", target) }
  function stop(target) { return lifecycle("stop", target) }
  function restart(target) { return lifecycle("restart", target) }
  function forceKill(target) { return lifecycle("kill", target) }
  function startAll() { return request("start", { target: "all" }) }
  function stopAll() { return request("stop", { target: "all" }) }

  // Visibility only adjusts the daemon's polling cadence. It is not a user
  // operation and must not produce the normal Working…/Done status toast.
  function setPanelVisible(value) { request("ui.visibility", { open: value === true }, true) }

  function createProject(project) { return request("project.create", { project: project }) }
  function updateProject(project) { return request("project.update", { project: project }) }
  function duplicateProject(projectId) { return request("project.duplicate", { projectId: String(projectId) }) }
  function deleteProject(projectId) { return request("project.delete", { projectId: String(projectId) }) }
  function reorderProjects(ids) { return request("project.reorder", { projectIds: ids }) }
  function createService(projectId, service) { return request("service.create", { projectId: String(projectId), service: service }) }
  function updateService(service) { return request("service.update", { projectId: "", service: service }) }
  function deleteService(serviceId) { return request("service.delete", { serviceId: String(serviceId) }) }
  function updateSettings(settings) { return request("settings.update", { settings: settings }) }
  function importCompose(path) {
    var value = String(path || "").trim()
    if (value.length === 0) return false
    root.composeImportPath = value
    root.composeImport = []
    return request("docker.import", { composeFile: value })
  }
  function dockerAction(serviceId, action) { return request("docker.action", { serviceId: String(serviceId), action: String(action) }) }
  function dockerTerminal(serviceId) { return request("docker.terminal", { serviceId: String(serviceId) }) }

  function pumpRequests() {
    if (requestProcess.running || root.requestQueue.length === 0) return
    var queue = root.requestQueue.slice()
    var request = queue.shift()
    root.requestQueue = queue
    root._stdout = ""
    root._stderr = ""
    root._activeRequestSilent = request.silent === true
    if (!root._activeRequestSilent) root.actionStatus = "Working…"
    root._activeRequestMethod = request.method
    requestProcess.command = ["omastack", "request", request.method, JSON.stringify(request.params)]
    requestProcess.running = true
  }

  function loadLogs(target, query) {
    if (!validTarget(target) || logsProcess.running) return
    root.logsTarget = String(target)
    root.logsLoading = true
    root._logsStdout = ""
    root._logsStderr = ""
    var configuredLines = root.snapshot && root.snapshot.settings ? Number(root.snapshot.settings.logBufferLines) : 2000
    if (!isFinite(configuredLines)) configuredLines = 2000
    configuredLines = Math.max(100, Math.min(50000, Math.round(configuredLines)))
    logsProcess.command = ["omastack", "logs", "--json", "--lines", String(configuredLines), "--query", String(query || ""), String(target)]
    logsProcess.running = true
  }

  function clearVisibleLogs() { root.logs = [] }

  function openPanel() {
    if (root.shell && typeof root.shell.summon === "function") root.shell.summon("david.omastack", "{}")
  }

  FileView {
    id: stateFile
    path: root.snapshotPath
    watchChanges: true
    printErrors: false
    onLoaded: root.parseSnapshot(text())
    onFileChanged: reload()
    onLoadFailed: {
      root.snapshot = ({ version: 1, connected: false, projects: [], runtime: {}, routes: [], diagnostics: [] })
      root.lastError = "OmaStack backend is not connected"
    }
  }

  Process {
    id: requestProcess
    running: false
    command: []
    stdout: StdioCollector { waitForEnd: true; onStreamFinished: root._stdout = text }
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root._stderr = text }
    onExited: function(exitCode, exitStatus) {
      if (exitCode === 0) {
        var parseFailed = false
        if (root._activeRequestMethod === "docker.import") {
          try {
            var imported = JSON.parse(root._stdout || "[]")
            root.composeImport = Array.isArray(imported) ? imported : []
            root.actionStatus = "Discovered " + root.composeImport.length + " Compose service" + (root.composeImport.length === 1 ? "" : "s")
          } catch (error) {
            root.composeImport = []
            root.lastError = "Could not parse Compose discovery results"
            parseFailed = true
          }
        } else if (!root._activeRequestSilent) root.actionStatus = "Done"
        if (!parseFailed) root.lastError = ""
        refreshDelay.restart()
      } else {
        var message = String(root._stderr || root._stdout || "OmaStack request failed").replace(/\s+/g, " ").trim()
        root.lastError = message.length > 240 ? message.substring(0, 237) + "…" : message
        root.actionStatus = ""
      }
      if (!root._activeRequestSilent) actionClear.restart()
      root._activeRequestSilent = false
      root.pumpRequests()
    }
  }

  Process {
    id: logsProcess
    running: false
    command: []
    stdout: StdioCollector { waitForEnd: true; onStreamFinished: root._logsStdout = text }
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root._logsStderr = text }
    onExited: function(exitCode, exitStatus) {
      root.logsLoading = false
      if (exitCode !== 0) { root.lastError = String(root._logsStderr || "Could not load logs").trim(); return }
      try { root.logs = JSON.parse(root._logsStdout || "[]") } catch (error) { root.logs = []; root.lastError = "Could not parse logs" }
    }
  }

  Timer { id: refreshDelay; interval: 350; onTriggered: root.refresh() }
  Timer { id: actionClear; interval: 1800; onTriggered: root.actionStatus = "" }
  Timer { interval: 15000; repeat: true; running: true; onTriggered: root.refresh() }
  Timer { interval: 5000; repeat: true; running: true; onTriggered: root._clockMs = Date.now() }

  IpcHandler {
    target: "david.omastack"
    function open(): string { root.openPanel(); return "ok" }
    function show(): string { root.openPanel(); return "ok" }
    function refresh(): string { root.refresh(); return "ok" }
    function status(): string { return JSON.stringify({ connected: root.connected, running: root.runningCount, stopped: root.stoppedCount, unhealthy: root.unhealthyCount, crashed: root.crashedCount }) }
    function start(target: string): string { return root.start(target) ? "queued" : "invalid" }
    function stop(target: string): string { return root.stop(target) ? "queued" : "invalid" }
    function restart(target: string): string { return root.restart(target) ? "queued" : "invalid" }
    function startAll(): string { root.startAll(); return "queued" }
    function stopAll(): string { root.stopAll(); return "queued" }
  }

  Component.onCompleted: refresh()
}
