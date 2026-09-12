import QtQuick
import QtTest
import qs.Commons
import "../../qml/components" as AppComponents
import "../../qml" as App

Rectangle {
  id: stage
  width: 380
  height: 650
  color: Color.popups.background

  Component { id: logsComponent; AppComponents.LogsView {} }
  Component { id: serviceRowComponent; AppComponents.CompactServiceRow {} }
  Component { id: serviceEditorComponent; AppComponents.ServiceEditor {} }
  Component { id: panelComponent; App.CompactPanel {} }
  Component { id: backendComponent; App.Service {} }
  Component { id: connectionComponent; App.BackendConnection {} }
  Component { id: spyComponent; SignalSpy {} }
  Component { id: wizardComponent; AppComponents.ProjectWizard {} }

  TestCase {
    name: "ReleaseRegression"
    when: windowShown

    function findChildWhere(item, predicate) {
      if (predicate(item)) return item
      var children = item.children || []
      for (var index = 0; index < children.length; index++) {
        var found = findChildWhere(children[index], predicate)
        if (found) return found
      }
      return null
    }

    function stubService() {
      return {
        connected:true, projects:[], runtime:{}, logs:[], logsLoading:false,
        lastError:"", actionStatus:"", composeImport:[],
        runningCount:0, stoppedCount:0, unhealthyCount:0, crashedCount:0,
        snapshot:{settings:{pollIntervalSeconds:2, historySamples:60, logBufferLines:2000,
          notifications:{crashed:true, unhealthy:true, recovered:true}, proxy:{enabled:false, listenHost:"127.0.0.1", httpPort:8088}}, routes:[], diagnostics:[]},
        setPanelVisible:function(value) {}, refresh:function() {}, updateSettings:function(value) {}
      }
    }

    function test_requestSecretsUseStdin() {
      var backend = createTemporaryObject(backendComponent, stage, {manageIpc:false})
      var secret = "sentinel-秘密\npassword"
      backend.createService("project", {name:"test", environment:{TOKEN:{value:secret,secret:true}}})
      var process = null
      for (var i=0; i<backend.data.length; i++) {
        var item = backend.data[i]
        if (item.command && item.command[1] === "request") process = item
      }
      verify(process !== null)
      compare(process.command[3], "-")
      verify(JSON.stringify(process.command).indexOf("sentinel") === -1)
      verify(process.stdinEnabled)
      process.started()
      compare(JSON.parse(process.written).service.environment.TOKEN.value, secret)
      compare(process.payload, "")
      backend.request("stop", {target:"fixture", secret:secret})
      var urgent = findChild(backend, "omastackUrgentRequest")
      verify(urgent !== null)
      compare(urgent.command[3], "-")
      verify(JSON.stringify(urgent.command).indexOf("sentinel") === -1)
      urgent.started()
      compare(JSON.parse(urgent.written).secret, secret)
      compare(urgent.requestData.params, null)
    }

    function test_routeShortcutIncludesProxyPort() {
      var service = stubService()
      service.snapshot.settings.proxy.enabled = true
      service.snapshot.routes = [{hostname:"app.localhost", active:true, target:"127.0.0.1:3000", https:false}]
      var panel = createTemporaryObject(panelComponent, stage, {width:380, height:620, service:service, section:"routes"})
      verify(panel)
      wait(100)
      var button = findChildWhere(panel, function(item) { return item.modelData && item.modelData.hostname === "app.localhost" && typeof item.clicked === "function" })
      verify(button !== null)
      button.clicked()
      var command = []
      for (var i=0; i<panel.data.length; i++) {
        var item = panel.data[i]
        if (item.command && item.command[0] === "xdg-open") command = item.command
      }
      console.log("Route browser command=" + JSON.stringify(command))
      compare(command[1], "http://app.localhost:8088", "Shortcut must address the actual proxy listener")
    }

    function test_settingsFitShortViewport() {
      var panel = createTemporaryObject(panelComponent, stage, {width:380, height:400, service:stubService(), section:"settings", advancedSettingsExpanded:true})
      verify(panel)
      wait(100)
      var scroll = findChildWhere(panel, function(item) { return item.visible && item.availableWidth !== undefined && item.availableHeight !== undefined && item.clip === true })
      verify(scroll !== null)
      var bottom = scroll.mapToItem(panel, 0, scroll.height).y
      console.log("Settings scroll area bottom=" + bottom + ", viewport=" + panel.height)
      verify(bottom <= panel.height, "Fixed-height settings scroll area extends below the fitted popup")
    }

    function test_themeLayoutCaptures() {
      var service = stubService()
      var web = {id:"web", name:"Web frontend", url:"http://127.0.0.1:3000", command:{executable:"/usr/bin/npm", arguments:["run","dev"]}}
      var api = {id:"api", name:"Application API", command:{executable:"/usr/bin/python", arguments:["server.py"]}}
      service.projects = [{id:"project", name:"Example project", icon:"󰆍", services:[web,api]}]
      var history = []
      for (var i=0;i<40;i++) history.push({cpu:Math.max(0, 12 + 15*Math.sin(i/4)),memoryMb:180+Math.sin(i/6)*10})
      service.runtime = {web:{status:"running",cpu:12.5,memoryMb:189,ports:[3000],history:history},api:{status:"crashed",cpu:0,memoryMb:0,lastError:"Address already in use: 127.0.0.1:8000"}}
      service.runningCount=1; service.crashedCount=1
      var panel = createTemporaryObject(panelComponent, stage, {width:380,height:500,service:service,expandedProjectId:"project",selectedServiceId:"web",panelVisible:true})
      verify(panel)
      Style.resolvedFontFamily = "JetBrainsMono Nerd Font"
      var themes = [
        {name:"everforest",raw:'mode="dark"\nbackground="#2d353b"\nforeground="#d3c6aa"\naccent="#7fbbb3"\nred="#e67e80"\nmuted="#475258"\nselection="#3d484d"'},
        {name:"tokyo-night",raw:'mode="dark"\nbackground="#1a1b26"\nforeground="#a9b1d6"\naccent="#7aa2f7"\nred="#f7768e"\nmuted="#414868"\nselection="#292e42"'},
        {name:"catppuccin-latte",raw:'mode="light"\nbackground="#eff1f5"\nforeground="#4c4f69"\naccent="#1e66f5"\nred="#d20f39"\nmuted="#acb0be"\nselection="#ccd0da"'}
      ]
      for (var t=0;t<themes.length;t++) {
        Color.loadColors(themes[t].raw)
        wait(150)
        panel.height = panel.implicitHeight
        wait(50)
        console.log("Fixture theme=" + themes[t].name + ", panel height=" + panel.height)
      }
    }

    function test_logMessageHasReadableWidth() {
      var view = createTemporaryObject(logsComponent, stage, {
        width: 380, height: 485, paused: true,
        entries: [{ timestamp: "2026-09-06T10:00:00.123Z", stream: "stdout", service: "Mauth Studio / Studio + MCP", message: "Server listening on http://127.0.0.1:3000" }]
      })
      verify(view)
      wait(100)
      var message = findChildWhere(view, function(item) { return item.readOnly === true && item.text === "Server listening on http://127.0.0.1:3000" })
      verify(message !== null, "Log delegate was not instantiated")
      console.log("Log message width=" + message.width + ", x=" + message.x + ", font=" + Style.font.caption)
      verify(message.width >= 120, "A normal service label leaves only " + message.width + "px for the log message")
    }

    function test_serviceControlsRemainVisibleWithKeyboardFocus() {
      var row = createTemporaryObject(serviceRowComponent, stage, {
        width: 360, expanded: true, serviceData: {id: "fixture", name: "Studio + MCP", url: "http://127.0.0.1:3000"},
        runtimeData: {status: "running", cpu: 1.5, memoryMb: 920, ports: [3000], history: []}
      })
      verify(row)
      mouseMove(stage, 379, 649)
      row.forceActiveFocus()
      wait(100)
      verify(row.actionsVisible, "Controls did not appear on row focus")
      var button = findChildWhere(row, function(item) { return item.tooltipText === "Open service in browser" })
      verify(button !== null)
      button.forceActiveFocus()
      wait(100)
      console.log("Focused browser button: visible=" + button.visible + ", actionsVisible=" + row.actionsVisible)
      verify(button.visible, "Controls vanish when focus moves from the row into an action")
    }

    function test_editorPreservesExistingConfiguration() {
      var editor = createTemporaryObject(serviceEditorComponent, stage, {width: 380, height: 620})
      verify(editor)
      editor.begin("project", {
        id: "service", name: "API", workingDirectory: "/tmp", command: {executable: "/usr/bin/true", arguments: []},
        docker: {composeFile: "/tmp/compose.yaml", projectName: "production-preview", service: "api"},
        restart: {mode: "on-failure", delaySeconds: 0, maxAttempts: 3, resetAfterSeconds: 300},
        health: {type: "http", intervalSeconds: 10, timeoutSeconds: 3, retries: 3, startGraceSeconds: 0, http: {url: "http://127.0.0.1:3000/health", expectedStatus: 200, responseRegex: "^ready$"}}
      })
      wait(100)
      var result = editor.buildService()
      console.log("Unchanged editor save=" + JSON.stringify({docker:result.docker, restart:result.restart, health:result.health}))
      compare(result.docker.projectName, "production-preview", "Unedited Compose project name must survive Save")
      compare(result.health.http.responseRegex, "^ready$")
      compare(result.restart.resetAfterSeconds, 300)
      compare(result.restart.delaySeconds, 0)
      compare(result.health.startGraceSeconds, 0)
    }

    function test_settingsEditsDoNotOverwriteEachOther() {
      var requests = []
      var service = {
        connected:true, projects:[], runtime:{}, logs:[], logsLoading:false,
        lastError:"", actionStatus:"", composeImport:[],
        runningCount:0, stoppedCount:0, unhealthyCount:0, crashedCount:0,
        snapshot: {settings: {pollIntervalSeconds:2, historySamples:60, logBufferLines:2000,
          notifications:{crashed:true, unhealthy:true, recovered:true}, proxy:{enabled:false, listenHost:"127.0.0.1", httpPort:8088}}, routes:[], diagnostics:[]},
        updateSettings: function(settings) { requests.push(settings) }
      }
      var panel = createTemporaryObject(panelComponent, stage, {width:380, height:620, service:service})
      verify(panel)
      panel.mutateSettings("notifications", "crashed", false)
      panel.mutateSettings("notifications", "unhealthy", false)
      console.log("Queued settings=" + JSON.stringify(requests))
      compare(requests[0].notifications.crashed, false)
      compare(requests[1].notifications.unhealthy, false)
      compare(requests[1].notifications.crashed, undefined, "Field patch must not overwrite an earlier edit")
    }

    function test_stopCanInterruptPendingStart() {
      var backend = createTemporaryObject(backendComponent, stage)
      verify(backend)
      backend.start("fixture")
      backend.stop("fixture")
      console.log("active=" + backend._activeRequestMethod + ", queued=" + JSON.stringify(backend.requestQueue))
      verify(backend._activeRequestMethod !== "start" || backend.requestQueue.length === 0,
             "Stop remains queued behind the long-running Start process")
    }

    function test_queuedAggregateStartsCannotUndoTargetedStop() {
      var backend = createTemporaryObject(backendComponent, stage)
      backend.snapshot = {projects:[{id:"project",name:"Project",services:[{id:"db",name:"DB"},{id:"web",name:"Web",dependencies:[{serviceId:"db",condition:"started"}]}]},{id:"other",name:"Other",services:[{id:"unrelated",name:"Worker"}]}]}
      verify(backend.startOverlapsStop("all", "web"))
      verify(backend.startOverlapsStop("project", "db"))
      verify(backend.startOverlapsStop("web", "db"))
      verify(!backend.startOverlapsStop("unrelated", "db"))
      backend.start("unrelated") // Occupy the normal FIFO.
      backend.startAll()
      backend.start("project")
      compare(backend.requestQueue.length, 2)
      backend.stop("db")
      compare(backend.requestQueue.length, 0)
    }

    function test_saveWaitsForMatchingAcknowledgmentAndKeepsFailedDraft() {
      var backend = createTemporaryObject(backendComponent, stage)
      var panel = createTemporaryObject(panelComponent, stage, {width:380,height:620,service:backend})
      var editor = createTemporaryObject(serviceEditorComponent, stage, {width:380,height:620})
      editor.begin("project", {id:"web",name:"Keep this draft",command:{executable:"/usr/bin/true"},workingDirectory:"/tmp"})
      panel.saveEditor(editor, "save-1")
      verify(editor.opened && editor.saving)
      backend.requestFinished("other", "service.update", true, "")
      verify(editor.opened && editor.saving)
      backend.requestFinished("save-1", "service.update", false, "Rejected fixture")
      verify(editor.opened && !editor.saving)
      compare(editor.saveError, "Rejected fixture")
      compare(editor.buildService().name, "Keep this draft")
      panel.saveEditor(editor, "save-2")
      backend.requestFinished("save-2", "service.update", true, "")
      verify(!editor.opened && !editor.saving)
    }

    function test_opaqueArgumentsAndIPv6RoundTrip() {
      var editor = createTemporaryObject(serviceEditorComponent, stage, {width:380,height:620})
      var args = ["first", "", "line\ninside", "last"]
      editor.begin("project", {id:"web",name:"IPv6",command:{executable:"/usr/bin/true",arguments:args},workingDirectory:"/tmp",health:{type:"tcp",intervalSeconds:10,timeoutSeconds:3,retries:1,startGraceSeconds:0,tcp:{host:"::1",port:8080}}})
      var result = editor.buildService()
      compare(JSON.stringify(result.command.arguments), JSON.stringify(args))
      compare(result.health.tcp.host, "::1")
      compare(result.health.tcp.port, 8080)
      compare(result.health.startGraceSeconds, 0)
    }

    function test_routeLookupCanonicalizesWhitespace() {
      var service = stubService()
      service.snapshot.routes = [{hostname:"app.localhost",active:true,url:"http://app.localhost:8088",target:"127.0.0.1:3000"}]
      var panel = createTemporaryObject(panelComponent, stage, {width:380,height:620,service:service})
      compare(panel.serviceRouteUrl({route:{hostname:" APP.localhost. "}}), "http://app.localhost:8088")
    }

    function test_wizardPreservesEmptyPositionalArguments() {
      var wizard = createTemporaryObject(wizardComponent, stage, {width:380,height:620,opened:true,page:1})
      var field = findChildWhere(wizard, function(item) { return item.placeholderText === "run\ndev\n--host\n127.0.0.1" })
      verify(field !== null)
      field.text = "first\n\nthird"
      var completed = createTemporaryObject(spyComponent, stage, {target:wizard,signalName:"completed"})
      wizard.finish()
      compare(completed.count, 1)
      compare(JSON.stringify(completed.signalArguments[0][0].services[0].command.arguments), '["first","","third"]')
      wizard.reset()
      compare(field.text, "")
    }

    function test_restrictedBarGetsOwnClientAndPanelProjects() {
      // This is the interface supplied by replacement bars: service lookup
      // exists but intentionally returns null for another plugin's service.
      var connection = createTemporaryObject(connectionComponent, stage, {
        hostShell: {serviceFor: function(id) { return null }}
      })
      verify(connection.service !== null)
      compare(connection.service.manageIpc, false, "Fallback must not register duplicate service IPC")
      var panel = createTemporaryObject(panelComponent, stage, {width:380,height:500,service:connection.service})
      connection.service.parseSnapshot(JSON.stringify({version:1,connected:true,
        generatedAt:new Date().toISOString(),projects:[{id:"fixture",name:"Saved app",services:[]}],runtime:{}}))
      compare(panel.connected, true)
      compare(panel.projects[0].name, "Saved app")
      connection.service.parseSnapshot("unreadable")
      compare(panel.projects[0].name, "Saved app", "Invalid reads must retain definitions")
      connection.service._clockMs = Date.now() + 100000
      compare(panel.connected, false)
      compare(panel.projects.length, 1)
      var shared = stubService()
      connection.hostShell = {serviceFor:function(id) { return shared }}
      compare(connection.service, shared, "Trusted hosts should reuse their shared service")
    }

    function test_startingRowOffersStop() {
      var row = createTemporaryObject(serviceRowComponent, stage, {width:360,serviceData:{id:"fixture",name:"Starting"},runtimeData:{status:"starting"}})
      var stop = findChildWhere(row, function(item) { return item.tooltipText === "Cancel startup / stop service" })
      verify(stop !== null && stop.enabled)
      var stopped = createTemporaryObject(spyComponent, stage, {target:row,signalName:"stopRequested"})
      stop.clicked()
      compare(stopped.count, 1)
    }

    function test_editorEscapeCancels() {
      var editor = createTemporaryObject(serviceEditorComponent, stage, {width:380, height:620})
      verify(editor)
      editor.begin("project", null)
      var canceled = createTemporaryObject(spyComponent, stage, {target:editor, signalName:"canceled"})
      editor.forceActiveFocus()
      keyClick(Qt.Key_Escape)
      compare(canceled.count, 1, "Escape should cancel the modal editor")
    }

    function test_longLogLabelDoesNotHideMessage() {
      var view = createTemporaryObject(logsComponent, stage, {
        width:380, height:485, paused:true,
        entries:[{timestamp:"2026-09-06T10:00:00.123Z", stream:"stderr",
          service:"Customer Management Development / Frontend + API Server", message:"Failed to connect to database"}]
      })
      verify(view)
      wait(100)
      var message = findChildWhere(view, function(item) { return item.readOnly === true && item.text === "Failed to connect to database" })
      verify(message !== null)
      console.log("Long-label log message width=" + message.width)
      verify(message.width > 0, "A valid label makes the log message width negative: " + message.width)
    }
  }
}
