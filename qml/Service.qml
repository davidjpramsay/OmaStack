import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  visible: false

  property var shell: null
  property var manifest: null
  property bool manageIpc: true
  property var snapshot: ({ version: 1, connected: false, projects: [], runtime: {}, routes: [], diagnostics: [] })
  property string lastError: ""
  property string actionStatus: ""
  property var logs: []
  property string logsTarget: ""
  property bool logsLoading: false
  property string logsNotice: ""
  property string maintenanceResult: ""
  property var composeImport: []
  property string composeImportPath: ""
  property double _clockMs: Date.now()

  readonly property string runtimeBase: Quickshell.env("XDG_RUNTIME_DIR") || ""
  readonly property string snapshotPath: runtimeBase !== "" ? runtimeBase + "/omastack/state.json" : ""
  readonly property string binaryPath: {
    var home = Quickshell.env("HOME") || ""
    return home !== "" ? home + "/.local/bin/omastack" : "omastack"
  }
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
  property string _statusStdout: ""
  property string _statusStderr: ""
  property string _logsStdout: ""
  property string _logsStderr: ""
  property string _activeRequestMethod: ""
  property bool _activeRequestSilent: false
  property string _activeRequestToken: ""
  property int _requestSequence: 0
  property int _urgentRunning: 0
  property var _pendingSettings: []
  property var _acknowledgedSettings: null
  property double _settingsAcknowledgedAt: 0
  readonly property var settings: mergedSettings()
  readonly property bool settingsPending: _pendingSettings.length > 0
  signal requestFinished(string token, string method, bool success, string message)

  function applySettingsPatch(base, patch) {
    var next = JSON.parse(JSON.stringify(base || {}))
    for (var key in patch) {
      if (patch[key] && typeof patch[key] === "object") {
        next[key] = next[key] || {}
        for (var field in patch[key]) next[key][field] = patch[key][field]
      } else next[key] = patch[key]
    }
    return next
  }

  function mergedSettings() {
    var next = _acknowledgedSettings || (snapshot && snapshot.settings) || {}
    for (var i=0; i<_pendingSettings.length; i++) next = applySettingsPatch(next, _pendingSettings[i].patch)
    return next
  }

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
      if (root.lastError === "OmaStack backend is not connected" || root.lastError === "Could not read the OmaStack runtime snapshot") root.lastError = ""
      if (Date.parse(parsed.generatedAt) >= root._settingsAcknowledgedAt) root._acknowledgedSettings = null
    } catch (error) {
      root.lastError = "Could not read the OmaStack runtime snapshot"
    }
  }

  function refresh() {
    if (root.snapshotPath !== "") stateFile.reload()
    if (!statusProcess.running) {
      root._statusStdout = ""
      root._statusStderr = ""
      statusProcess.running = true
    }
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
    var token = String(++root._requestSequence)
    var item = { token: token, method: String(method), params: params || {}, silent: silent === true }
    if (method === "stop" || method === "kill" || method === "restart") {
      if (root._urgentRunning >= 4) { root.lastError = "Service controls are busy; try again shortly"; return false }
      root.requestQueue = root.requestQueue.filter(function(pending) {
        return !((pending.method === "start" || pending.method === "restart") && root.startOverlapsStop(pending.params.target, params.target))
      })
      var process = urgentRequestComponent.createObject(root, { requestData: item })
      if (!process) { root.lastError = "Could not start service control request"; return false }
      root._urgentRunning++
      root.actionStatus = "Working…"
      process.running = true
      return token
    }
    var next = root.requestQueue.slice()
    next.push(item)
    root.requestQueue = next
    pumpRequests()
    return token
  }

  function selectedServiceIds(target, includeDependencies) {
    if (target === "all") return null
    var selected = [], definitions = {}
    for (var pi=0; pi<projects.length; pi++) {
      var project = projects[pi], items = project.services || []
      for (var si=0; si<items.length; si++) {
        var item = items[si]
        definitions[item.id] = item
        if (target === project.id || target === project.name || target === item.id || target === item.name || target === project.name + "/" + item.name) selected.push(item.id)
      }
    }
    if (!selected.length) return null // Unknown/ambiguous scope: cancel conservatively.
    var seen = {}
    for (var i=0; i<selected.length; i++) {
      var id = selected[i]
      if (seen[id]) continue
      seen[id] = true
      var dependencies = includeDependencies && definitions[id] ? definitions[id].dependencies || [] : []
      for (var j=0; j<dependencies.length; j++) if (!seen[dependencies[j].serviceId]) selected.push(dependencies[j].serviceId)
    }
    return Object.keys(seen)
  }

  function startOverlapsStop(startTarget, stopTarget) {
    var startIds = selectedServiceIds(String(startTarget), true)
    var stopIds = selectedServiceIds(String(stopTarget), false)
    if (!startIds || !stopIds) return true
    return startIds.some(function(id) { return stopIds.indexOf(id) >= 0 })
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
  function updateSettings(patch) {
    var token = request("settings.patch", patch)
    if (token) root._pendingSettings = root._pendingSettings.concat([{token:token, patch:patch}])
    return token
  }
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
    root._activeRequestToken = request.token
    requestProcess.payload = JSON.stringify(request.params)
    requestProcess.command = [root.binaryPath, "request", request.method, "-"]
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
    logsProcess.command = [root.binaryPath, "logs", "--json", "--metadata", "--lines", String(configuredLines), "--query", String(query || ""), String(target)]
    logsProcess.running = true
  }

  function clearVisibleLogs() { root.logs = [] }

  function openPanel() {
    if (root.shell && typeof root.shell.summon === "function") root.shell.summon("david.omastack", "{}")
  }

  function finishRequest(token, method, silent, code, stdout, stderr) {
    var success = code === 0
    var result = null
    var message = ""
    if (success) {
      try { result = JSON.parse(stdout || "null") }
      catch (error) { success = false; message = "Could not parse OmaStack response" }
    }
    if (!success && message === "") message = String(stderr || stdout || "OmaStack request failed").replace(/\s+/g, " ").trim().substring(0,512)
    if (method === "settings.patch") {
      if (success && result) {
        root._acknowledgedSettings = result
        root._settingsAcknowledgedAt = Date.now()
      }
      root._pendingSettings = root._pendingSettings.filter(function(item) { return item.token !== token })
    }
    if (success) {
      if (method === "docker.import") {
        root.composeImport = Array.isArray(result) ? result : []
        root.actionStatus = "Discovered " + root.composeImport.length + " Compose services"
      } else if (!silent) root.actionStatus = result && result.notice ? String(result.notice) : "Done"
      if (method === "doctor" || method === "cleanup" || method === "config.export") root.maintenanceResult = JSON.stringify(result, null, 2)
      if (!silent) root.lastError = ""
      refreshDelay.restart()
    } else {
      root.lastError = message
      root.actionStatus = ""
    }
    root.requestFinished(token, method, success, message)
    if (!silent) actionClear.restart()
  }

  Component {
    id: urgentRequestComponent
    Process {
      id: urgent
      objectName: "omastackUrgentRequest"
      required property var requestData
      property string output: ""
      property string errors: ""
      command: [root.binaryPath, "request", requestData.method, "-"]
      stdinEnabled: true
      onStarted: {
        write(JSON.stringify(requestData.params) + "\n")
        requestData.params = null
      }
      stdout: StdioCollector { waitForEnd: true; onStreamFinished: urgent.output = text }
      stderr: StdioCollector { waitForEnd: true; onStreamFinished: urgent.errors = text }
      onExited: function(code) {
        root._urgentRunning--
        root.finishRequest(requestData.token, requestData.method, false, code, output, errors)
        Qt.callLater(function() { urgent.destroy() })
      }
    }
  }

  FileView {
    id: stateFile
    path: root.snapshotPath
    watchChanges: true
    printErrors: false
    onLoaded: root.parseSnapshot(text())
    onFileChanged: reload()
    onLoadFailed: {
      // Keep the last valid snapshot visible during transient file-read
      // failures. The freshness check on `connected` still marks it offline,
      // but a reload hiccup must not make configured projects disappear.
      root.lastError = "OmaStack backend is not connected"
    }
  }

  Process {
    id: requestProcess
    property string payload: ""
    stdinEnabled: true
    onStarted: {
      write(payload + "\n")
      payload = ""
    }
    running: false
    command: []
    stdout: StdioCollector { waitForEnd: true; onStreamFinished: root._stdout = text }
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root._stderr = text }
    onExited: function(exitCode, exitStatus) {
      root.finishRequest(root._activeRequestToken, root._activeRequestMethod, root._activeRequestSilent, exitCode, root._stdout, root._stderr)
      root._activeRequestSilent = false
      root.pumpRequests()
    }
  }

  // A direct status request also recovers from missed file-watch updates.
  Process {
    id: statusProcess
    running: false
    command: [root.binaryPath, "status", "--json"]
    stdout: StdioCollector { waitForEnd: true; onStreamFinished: root._statusStdout = text }
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root._statusStderr = text }
    onExited: function(exitCode) {
      if (exitCode === 0) root.parseSnapshot(root._statusStdout)
      else if (!root.snapshot || !root.snapshot.generatedAt) root.lastError = "OmaStack backend is not connected"
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
      try {
        var result = JSON.parse(root._logsStdout || "{}")
        root.logs = Array.isArray(result) ? result : (result.entries || [])
        root.logsNotice = result.notice || ""
      } catch (error) { root.logs = []; root.lastError = "Could not parse logs" }
    }
  }

  Timer { id: refreshDelay; interval: 350; onTriggered: root.refresh() }
  Timer { id: actionClear; interval: 1800; onTriggered: root.actionStatus = "" }
  Timer { interval: 15000; repeat: true; running: true; onTriggered: root.refresh() }
  Timer { interval: 5000; repeat: true; running: true; onTriggered: root._clockMs = Date.now() }

  IpcHandler {
    enabled: root.manageIpc
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
