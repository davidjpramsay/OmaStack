import QtQuick
import QtQuick.Window
import QtQuick.Controls as Controls
import QtQuick.Layouts as Layouts
import Quickshell.Io
import qs.Commons
import qs.Ui
import "components"

Item {
  id: root

  property var service: null
  property bool panelVisible: false
  property bool showStatusCounts: true
  property string section: "stack"
  property string selectedProjectId: ""
  property string expandedProjectId: ""
  property string selectedServiceId: ""
  property string searchText: ""
  property string pendingDeleteType: ""
  property string pendingDeleteId: ""
  property string returnSection: "stack"
  property int keyboardProjectIndex: 0
  property bool keyboardCursorActive: false
  property bool advancedSettingsExpanded: false
  property var pendingEditor: null
  property string pendingSaveToken: ""

  signal closeRequested()
  signal switchRequested(int direction)
  signal showStatusCountsRequested(bool value)

  readonly property alias focusTarget: keyCatcher
  readonly property var projects: service ? service.projects : []
  readonly property var runtime: service ? service.runtime : ({})
  readonly property var selectedProject: projectById(selectedProjectId)
  readonly property var selectedService: serviceById(selectedServiceId)
  readonly property bool connected: Boolean(service && service.connected)
  readonly property int visibleProjectCount: projectList.count
  readonly property var settings: service && service.settings ? service.settings : (service && service.snapshot.settings ? service.snapshot.settings : ({}))
  readonly property color primaryText: Color.popups.text
  readonly property color secondaryText: Qt.tint(Color.popups.background, Util.alpha(primaryText, 0.82))
  readonly property bool editorOpen: projectWizard.opened || projectEditor.opened || serviceEditor.opened || deleteDialog.opened || importDialog.opened
  readonly property int stackListHeight: estimatedStackListHeight()
  readonly property int preferredWidth: 380
  readonly property real stackChromeHeight: stackHero.implicitHeight + counts.height + searchField.implicitHeight + stackSeparatorTop.height + stackSeparatorBottom.height + stackFooter.height + stackColumn.spacing*6
  readonly property real preferredHeight: editorOpen
    ? Style.space(620)
    : (section === "stack" ? stackChromeHeight + Style.space(stackListHeight)
      : (section === "settings" ? settingsPage.implicitHeight : Style.space(590)))
  implicitHeight: preferredHeight

  function prepareOpen(payloadJson) {
    keyboardCursorActive = false
    if (service) { service.setPanelVisible(true); service.refresh() }
    if (payloadJson) {
      try {
        var payload = JSON.parse(String(payloadJson))
        if (payload && payload.section) section = String(payload.section)
        if (payload && payload.project) selectedProjectId = String(payload.project)
        if (payload && payload.service) selectedServiceId = String(payload.service)
      } catch (error) { /* Optional summon payload. */ }
    }
    ensureSelection()
    Qt.callLater(function() { keyCatcher.forceActiveFocus() })
  }

  function prepareClose() {
    if (service) service.setPanelVisible(false)
    searchField.focus = false
  }

  function requestBackOrClose() {
    if (editorOpen) return
    if (section !== "stack") { section = returnSection || "stack"; Qt.callLater(function() { keyCatcher.forceActiveFocus() }); return }
    root.closeRequested()
  }

  function projectById(id) {
    for (var index = 0; index < projects.length; index++) if (projects[index].id === id) return projects[index]
    return null
  }

  function serviceById(id) {
    for (var pi = 0; pi < projects.length; pi++) {
      var items = projects[pi].services || []
      for (var si = 0; si < items.length; si++) if (items[si].id === id) return items[si]
    }
    return null
  }

  function allServices() {
    var result = []
    for (var pi = 0; pi < projects.length; pi++) {
      var items = projects[pi].services || []
      for (var si = 0; si < items.length; si++) {
        var item = JSON.parse(JSON.stringify(items[si]))
        item.projectName = projects[pi].name
        result.push(item)
      }
    }
    return result
  }

  function ensureSelection() {
    if (projects.length === 0) {
      selectedProjectId = ""; expandedProjectId = ""; selectedServiceId = ""; return
    }
    if (!projectById(selectedProjectId)) selectedProjectId = projects[0].id
    // An empty ID is the intentional "all projects collapsed" state. Only
    // repair a non-empty ID when its project was actually removed.
    if (expandedProjectId !== "" && !projectById(expandedProjectId)) expandedProjectId = selectedProjectId
    var project = projectById(selectedProjectId)
    if (selectedServiceId !== "" && (!serviceById(selectedServiceId) || (project && (project.services || []).every(function(item) { return item.id !== selectedServiceId }))))
      selectedServiceId = ""
  }

  function estimatedStackListHeight() {
    var visibleProjects = filteredProjects()
    if (visibleProjects.length === 0) return 120
    var total = 0
    for (var index = 0; index < visibleProjects.length; index++) {
      var project = visibleProjects[index]
      total += 38
      if (project.id === expandedProjectId) {
        var visibleServices = filteredServices(project)
        for (var si=0; si<visibleServices.length; si++) {
          var svc = visibleServices[si]
          var value = runtime[svc.id] || {}
          total += (selectedServiceId === svc.id ? 82 : 34) + 3
          if ((value.status === "crashed" || value.status === "unhealthy") && value.lastError) total += 18
        }
        if (visibleServices.length === 0) total += 38
      }
      if (index > 0) total += 4
    }
    return Math.max(42, Math.min(total, 390))
  }

  function filteredProjects() {
    var needle = searchText.trim().toLowerCase()
    if (!needle) return projects
    return projects.filter(function(project) {
      if (String(project.name || "").toLowerCase().indexOf(needle) >= 0) return true
      var items = project.services || []
      for (var i = 0; i < items.length; i++) {
        if (String(items[i].name || "").toLowerCase().indexOf(needle) >= 0 || String(items[i].description || "").toLowerCase().indexOf(needle) >= 0) return true
      }
      return false
    })
  }

  function filteredServices(project) {
    if (!project) return []
    var items = project.services || []
    var needle = searchText.trim().toLowerCase()
    if (!needle || String(project.name || "").toLowerCase().indexOf(needle) >= 0) return items
    return items.filter(function(item) {
      return String(item.name || "").toLowerCase().indexOf(needle) >= 0 || String(item.description || "").toLowerCase().indexOf(needle) >= 0
    })
  }

  function projectState(project) {
    if (!project || !project.services || project.services.length === 0) return "stopped"
    var running = 0, stopped = 0
    for (var i = 0; i < project.services.length; i++) {
      var value = runtime[project.services[i].id] || ({ status: "stopped" })
      if (value.status === "crashed") return "crashed"
      if (value.status === "unhealthy") return "unhealthy"
      if (value.status === "starting" || value.status === "stopping") return value.status
      if (value.status === "running") running++
      else stopped++
    }
    if (running === project.services.length) return "running"
    if (stopped === project.services.length) return "stopped"
    return "partial"
  }

  function stateLabel(state) {
    if (state === "running") return "Running"
    if (state === "stopped") return "Stopped"
    if (state === "unhealthy") return "Unhealthy"
    if (state === "crashed") return "Crashed"
    if (state === "starting") return "Starting"
    if (state === "stopping") return "Stopping"
    return "Partial"
  }

  function stateTextColor(state) {
    if (state === "crashed" || state === "unhealthy") return Color.urgent
    if (state === "running" || state === "starting" || state === "stopping") return Color.accent
    return secondaryText
  }

  function openProject(project) {
    selectedProjectId = project.id
    expandedProjectId = expandedProjectId === project.id ? "" : project.id
    ensureSelection()
  }

  function moveProjectCursor(delta) {
    var items = filteredProjects()
    if (items.length === 0) { keyboardProjectIndex = 0; return }
    keyboardCursorActive = true
    keyboardProjectIndex = Math.max(0, Math.min(items.length - 1, keyboardProjectIndex + delta))
    selectedProjectId = items[keyboardProjectIndex].id
  }

  function activateProjectCursor() {
    var items = filteredProjects()
    if (items.length === 0) return
    keyboardCursorActive = true
    keyboardProjectIndex = Math.max(0, Math.min(items.length - 1, keyboardProjectIndex))
    openProject(items[keyboardProjectIndex])
  }

  function selectService(projectId, serviceId) {
    selectedProjectId = projectId
    selectedServiceId = serviceId
  }

  function showLogs(target, projectId) {
    if (!service) return
    returnSection = "stack"
    if (projectId) selectedProjectId = projectId
    if (serviceById(target)) selectedServiceId = target
    section = "logs"
    service.loadLogs(target, "")
  }

  function openBrowserUrl(value) {
    var target = String(value || "").trim()
    if (!(target.indexOf("http://") === 0 || target.indexOf("https://") === 0)) {
      if (service) service.lastError = "Only HTTP and HTTPS service URLs can be opened"
      return
    }
    if (openUrlProcess.running) return
    openUrlProcess.command = ["xdg-open", target]
    openUrlProcess.running = true
  }

  function routeUrl(route) {
    if (route.url) return route.url
    return (route.https ? "https://" : "http://") + route.hostname + ":" + (route.https ? (settings.proxy || {}).httpsPort : (settings.proxy || {}).httpPort)
  }

  function serviceRouteUrl(serviceData) {
    if (!serviceData.route || !service || !service.snapshot.routes) return ""
    var host = String(serviceData.route.hostname).trim().toLowerCase().replace(/\.$/, "")
    for (var i=0; i<service.snapshot.routes.length; i++) {
      var route = service.snapshot.routes[i]
      if (route.active && route.hostname === host) return routeUrl(route)
    }
    return ""
  }

  function focusWithin(item) {
    var focus = root.Window.window ? root.Window.window.activeFocusItem : null
    while (focus) { if (focus === item) return true; focus = focus.parent }
    return false
  }

  function saveEditor(editor, token) {
    if (!token) { editor.saveError = service.lastError || "Could not queue save"; return }
    editor.saving = true
    editor.saveError = ""
    pendingEditor = editor
    pendingSaveToken = String(token)
  }

  function mutateSettings(sectionName, key, value) {
    if (!service || !service.snapshot || !service.snapshot.settings) return
    var patch = {}
    if (sectionName === "") patch[key] = value
    else { patch[sectionName] = {}; patch[sectionName][key] = value }
    service.updateSettings(patch)
  }

  function requestDelete(type, id) {
    pendingDeleteType = type; pendingDeleteId = id
    deleteDialog.confirmText = type === "kill" ? "Force kill" : "Delete"
    deleteDialog.message = type === "kill" ? "Force-kill this service and its child processes? Unsaved application data may be lost."
      : type === "project"
      ? "Delete this project and its service definitions? Running services must be stopped first."
      : "Delete this service definition? This is refused while another service depends on it."
    deleteDialog.opened = true
  }

  function addComposeService(discovered) {
    if (!service || !selectedProject || !discovered) return
    service.createService(selectedProject.id, {
      name: String(discovered.name || "Compose service"), description: "Imported from " + service.composeImportPath,
      command: { executable: "", arguments: [] }, workingDirectory: "", environment: {}, autostart: false,
      restart: { mode: "never", delaySeconds: 1, maxAttempts: 0, resetAfterSeconds: 60 },
      stopSignal: "SIGTERM", gracefulStopSeconds: 20, dependencies: [], notes: "",
      docker: { composeFile: service.composeImportPath, projectName: "", service: String(discovered.name || "") }
    })
  }

  onProjectsChanged: ensureSelection()

  Item {
    id: keyCatcher
    anchors.fill: parent
    focus: true
    Keys.onPressed: function(event) {
      if (deleteDialog.opened && deleteDialog.handleKey(event)) { event.accepted = true; return }
      if (importDialog.opened && importDialog.handleKey(event)) { event.accepted = true; return }
      if (root.editorOpen) return
      if (event.key === Qt.Key_Escape) { root.requestBackOrClose(); event.accepted = true; return }
      // Leave Tab/Backtab and focused controls' keys to Qt's real focus chain.
      if (!activeFocus || event.key === Qt.Key_Tab || event.key === Qt.Key_Backtab) return
      if (event.key === Qt.Key_Down) { root.moveProjectCursor(1); event.accepted = true }
      else if (event.key === Qt.Key_Up) { root.moveProjectCursor(-1); event.accepted = true }
      else if (event.key === Qt.Key_Return || event.key === Qt.Key_Space) { root.activateProjectCursor(); event.accepted = true }
      else if (event.text === "/") { searchField.forceActiveFocus(); event.accepted = true }
      else if (event.text === "a" || event.text === "A") { projectWizard.reset(); projectWizard.opened = true; event.accepted = true }
      else if (event.text === "r" || event.text === "R") { if (root.service) root.service.refresh(); event.accepted = true }
    }

    Layouts.StackLayout {
      anchors.fill: parent
      currentIndex: root.section === "routes" ? 1 : (root.section === "settings" ? 2 : (root.section === "logs" ? 3 : 0))

      Item {
        Column {
          id: stackColumn
          width: parent.width
          spacing: Style.space(10)

          PanelHero {
            id: stackHero
            width: parent.width
            title: "OmaStack"
            meta: root.connected ? "Service manager" : "Backend disconnected"
            foreground: Color.foreground
            fontFamily: Style.font.family
            iconComponent: Component {
              Text { textFormat: Text.PlainText; text: "󰆍"; color: root.connected ? Color.accent : Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.display }
            }
            trailingControl: Component {
              Row {
                spacing: Style.space(2)
                PanelActionButton { iconText: "󰐕"; tooltipText: "Add project"; focusable: true; onClicked: { projectWizard.reset(); projectWizard.opened = true } }
                PanelActionButton { iconText: "󰒓"; tooltipText: "Settings and diagnostics"; focusable: true; onClicked: root.section = "settings" }
              }
            }
          }

          Row {
            id: counts
            width: parent.width
            spacing: Style.space(4)
            readonly property var values: [
              { label: "up", value: root.service ? root.service.runningCount : 0, state: "running" },
              { label: "down", value: root.service ? root.service.stoppedCount : 0, state: "stopped" },
              { label: "warn", value: root.service ? root.service.unhealthyCount : 0, state: "unhealthy" },
              { label: "failed", value: root.service ? root.service.crashedCount : 0, state: "crashed" }
            ]

            Repeater {
              model: counts.values
              Item {
                required property var modelData
                width: (counts.width - counts.spacing * 3) / 4
                height: Style.space(24)
                Row {
                  anchors.centerIn: parent
                  spacing: Style.space(4)
                  StatusDot { anchors.verticalCenter: parent.verticalCenter; status: modelData.state; dotSize: Style.space(4) }
                  Text { textFormat: Text.PlainText; text: modelData.value; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
                  Text { textFormat: Text.PlainText; text: modelData.label; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                }
              }
            }
          }

          TextField {
            id: searchField
            width: parent.width
            placeholderText: "Search projects and services…"
            Accessible.name: "Search projects and services"
            onTextChanged: root.searchText = text
            Keys.onEscapePressed: {
              if (text !== "") text = ""
              else { focus = false; keyCatcher.forceActiveFocus() }
            }
          }

          PanelSeparator { id: stackSeparatorTop; width: parent.width }

          ListView {
            id: projectList
            width: parent.width
            height: Math.max(0, Math.min(Style.space(root.stackListHeight), root.height-root.stackChromeHeight))
            model: root.filteredProjects()
            spacing: Style.space(4)
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            Controls.ScrollBar.vertical: Controls.ScrollBar {}

            delegate: Item {
              id: projectDelegate
              required property int index
              required property var modelData
              width: projectList.width
              height: projectContent.implicitHeight
              readonly property bool expanded: root.expandedProjectId === modelData.id
              readonly property string status: root.projectState(modelData)
              readonly property bool runningState: status === "running" || status === "unhealthy" || status === "partial"

              Column {
                id: projectContent
                width: parent.width
                spacing: Style.space(3)

                Button {
                  id: projectHeader
                  width: parent.width
                  height: Style.space(38)
                  leftAlign: true
                  focusable: true
                  selected: false
                  bordered: projectDelegate.expanded
                  background: projectDelegate.expanded ? Style.selectedFillFor(root.primaryText, Color.accent, Color.urgent) : "transparent"
                  foreground: root.primaryText
                  hasCursor: root.keyboardCursorActive && projectDelegate.index === root.keyboardProjectIndex
                  horizontalPadding: 0
                  verticalPadding: 0
                  text: ""
                  iconText: ""
                  onClicked: root.openProject(projectDelegate.modelData)
                  onRightClicked: projectEditor.begin(projectDelegate.modelData)

                  HoverHandler { id: projectHover }

                  Item {
                    anchors.fill: parent
                    anchors.leftMargin: Style.space(7)
                    anchors.rightMargin: Style.space(6)

                    BorderSurface {
                      id: projectIcon
                      anchors.left: parent.left
                      anchors.verticalCenter: parent.verticalCenter
                      width: Style.space(25); height: width
                      color: Util.alpha(projectStateDot.statusColor, 0.10)
                      borderSpec: Border.flat(Util.alpha(projectStateDot.statusColor, 0.32), 1)
                      radius: Style.cornerRadius
                      Text { textFormat: Text.PlainText; anchors.centerIn: parent; text: projectDelegate.modelData.icon || "󰆍"; color: projectStateDot.statusColor; font.family: Style.font.family; font.pixelSize: Style.font.body }
                    }

                    Column {
                      anchors.left: projectIcon.right
                      anchors.leftMargin: Style.space(8)
                      anchors.right: projectActions.visible ? projectActions.left : projectSummary.left
                      anchors.rightMargin: Style.space(8)
                      anchors.verticalCenter: parent.verticalCenter
                      spacing: Style.space(1)
                      Text { textFormat: Text.PlainText; width: parent.width; text: projectDelegate.modelData.name || "Untitled project"; color: root.primaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true; elide: Text.ElideRight }
                      Text { textFormat: Text.PlainText; width: parent.width; text: (projectDelegate.modelData.services || []).length + " service" + ((projectDelegate.modelData.services || []).length === 1 ? "" : "s"); color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption; elide: Text.ElideRight }
                    }

                    Row {
                      id: projectSummary
                      visible: !projectActions.visible
                      anchors.right: chevron.left
                      anchors.rightMargin: Style.space(7)
                      anchors.verticalCenter: parent.verticalCenter
                      spacing: Style.space(5)
                      StatusDot { id: projectStateDot; anchors.verticalCenter: parent.verticalCenter; status: projectDelegate.status; dotSize: Style.space(5) }
                      Text { textFormat: Text.PlainText; text: root.stateLabel(projectDelegate.status); color: root.stateTextColor(projectDelegate.status); font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true }
                    }

                    Row {
                      id: projectActions
                      visible: projectHover.hovered || root.focusWithin(projectHeader)
                      anchors.right: chevron.left
                      anchors.rightMargin: Style.space(4)
                      anchors.verticalCenter: parent.verticalCenter
                      spacing: Style.space(1)
                      PanelActionButton { iconText: projectDelegate.runningState || projectDelegate.status === "starting" ? "󰓛" : "󰐊"; tooltipText: projectDelegate.runningState || projectDelegate.status === "starting" ? "Stop project" : "Start project"; focusable: true; onClicked: projectDelegate.runningState || projectDelegate.status === "starting" ? root.service.stop(projectDelegate.modelData.id) : root.service.start(projectDelegate.modelData.id) }
                      PanelActionButton { iconText: "󰆍"; tooltipText: "Project logs"; focusable: true; onClicked: root.showLogs(projectDelegate.modelData.id, projectDelegate.modelData.id) }
                      PanelActionButton { iconText: "󰆏"; tooltipText: "Duplicate project"; focusable: true; onClicked: root.service.duplicateProject(projectDelegate.modelData.id) }
                      PanelActionButton { iconText: "󰏫"; tooltipText: "Edit project"; focusable: true; onClicked: projectEditor.begin(projectDelegate.modelData) }
                      PanelActionButton { iconText: "󰐕"; tooltipText: "Add service"; focusable: true; onClicked: { root.selectedProjectId = projectDelegate.modelData.id; serviceEditor.begin(projectDelegate.modelData.id, null) } }
                      PanelActionButton { iconText: "󰆴"; tooltipText: "Delete project"; hoverColor: Color.urgent; focusable: true; onClicked: root.requestDelete("project", projectDelegate.modelData.id) }
                    }

                    Text { textFormat: Text.PlainText; id: chevron; anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter; text: projectDelegate.expanded ? "󰅀" : "󰅂"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall }
                  }
                }

                Item {
                  visible: projectDelegate.expanded
                  width: parent.width
                  height: serviceGroup.implicitHeight

                  Rectangle {
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: Style.space(3)
                    width: Math.max(1, Style.normalBorderWidth)
                    color: Util.alpha(projectStateDot.statusColor, 0.52)
                  }

                  Column {
                    id: serviceGroup
                    x: Style.space(9)
                    width: parent.width - x
                    spacing: Style.space(3)

                    Repeater {
                      model: root.filteredServices(projectDelegate.modelData)
                      CompactServiceRow {
                        required property var modelData
                        width: serviceGroup.width
                        serviceData: modelData
                        runtimeData: root.runtime[modelData.id] || ({ status: "stopped", cpu: 0, memoryMb: 0, ports: [], history: [] })
                        expanded: root.selectedServiceId === modelData.id
                        panelVisible: root.panelVisible
                        historySamples: Number(root.settings.historySamples || 60)
                        routeUrl: root.serviceRouteUrl(modelData)
                        primaryText: root.primaryText
                        secondaryText: root.secondaryText
                        onClicked: {
                          var wasExpanded = root.selectedServiceId === modelData.id
                          root.selectService(projectDelegate.modelData.id, modelData.id)
                          root.selectedServiceId = wasExpanded ? "" : modelData.id
                        }
                        onStartRequested: function(id) { root.service.start(id) }
                        onStopRequested: function(id) { root.service.stop(id) }
                        onRestartRequested: function(id) { root.service.restart(id) }
                        onLogsRequested: function(id) { root.showLogs(id, projectDelegate.modelData.id) }
                        onEditRequested: function(id) { serviceEditor.begin(projectDelegate.modelData.id, root.serviceById(id)) }
                        onDeleteRequested: function(id) { root.requestDelete("service", id) }
                        onForceKillRequested: function(id) { root.requestDelete("kill", id) }
                        onOpenRequested: function(url) { root.openBrowserUrl(url) }
                        onDockerActionRequested: function(id, action) { root.service.dockerAction(id, action) }
                        onDockerTerminalRequested: function(id) { root.service.dockerTerminal(id) }
                      }
                    }

                    Button {
                      visible: (projectDelegate.modelData.services || []).length === 0
                      width: parent.width
                      text: "Add the first service"
                      iconText: "󰐕"
                      focusable: true
                      onClicked: serviceEditor.begin(projectDelegate.modelData.id, null)
                    }
                  }
                }
              }
            }

            Column {
              anchors.centerIn: parent
              visible: projectList.count === 0
              spacing: Style.space(7)
              Text { textFormat: Text.PlainText; anchors.horizontalCenter: parent.horizontalCenter; text: root.projects.length === 0 ? "󰆍" : "󰍉"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.display }
              Text { textFormat: Text.PlainText; anchors.horizontalCenter: parent.horizontalCenter; text: !root.connected ? "Waiting for the backend. Your saved projects are preserved." : (root.projects.length === 0 ? "No projects yet" : "No matching projects"); width: projectList.width - Style.space(24); horizontalAlignment: Text.AlignHCenter; wrapMode: Text.WordWrap; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
              Button { visible: root.projects.length === 0 && root.connected; anchors.horizontalCenter: parent.horizontalCenter; text: "Add project"; iconText: "󰐕"; focusable: true; onClicked: { projectWizard.reset(); projectWizard.opened = true } }
            }
          }

          PanelSeparator { id: stackSeparatorBottom; width: parent.width }

          Row {
            id: stackFooter
            width: parent.width
            height: Style.space(28)
            spacing: Style.space(4)
            Text { textFormat: Text.PlainText; anchors.verticalCenter: parent.verticalCenter; width: parent.width - footerActions.width - Style.space(4); text: root.projects.length + " project" + (root.projects.length === 1 ? "" : "s"); color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
            Row {
              id: footerActions
              anchors.verticalCenter: parent.verticalCenter
              spacing: Style.space(2)
              PanelActionButton { iconText: "󰐊"; tooltipText: "Start all services"; focusable: true; onClicked: if (root.service) root.service.startAll() }
              PanelActionButton { iconText: "󰓛"; tooltipText: "Stop all services"; focusable: true; onClicked: if (root.service) root.service.stopAll() }
              PanelActionButton { iconText: "󰖟"; tooltipText: "Local routes"; focusable: true; onClicked: root.section = "routes" }
              PanelActionButton { iconText: "󰑐"; tooltipText: "Refresh"; focusable: true; onClicked: if (root.service) root.service.refresh() }
            }
          }
        }
      }

      Item {
        Column {
          anchors.fill: parent
          spacing: Style.space(10)
          Row {
            width: parent.width; height: Style.space(28); spacing: Style.space(7)
            PanelActionButton { iconText: "󰁍"; tooltipText: "Back to projects"; focusable: true; onClicked: root.section = "stack" }
            Text { textFormat: Text.PlainText; anchors.verticalCenter: parent.verticalCenter; text: "Local routes"; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.title; font.bold: true }
          }
          Text { textFormat: Text.PlainText; width: parent.width; text: "Loopback routes only. OmaStack never changes DNS or trust silently."; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
          PanelSeparator { width: parent.width }
          ListView {
            id: routeList
            width: parent.width; height: Math.max(0, parent.height-y); clip: true
            Controls.ScrollBar.vertical: Controls.ScrollBar {}
            model: root.service && root.service.snapshot.routes ? root.service.snapshot.routes : []
            spacing: Style.space(4)
            delegate: Button {
              required property var modelData
              width: routeList.width; height: Style.space(48); leftAlign: true; focusable: true
              onClicked: if (modelData.active) root.openBrowserUrl(root.routeUrl(modelData))
              Item {
                anchors.fill: parent; anchors.margins: Style.space(8)
                StatusDot { id: routeDot; anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter; status: modelData.active ? "running" : "crashed" }
                Column { anchors.left: routeDot.right; anchors.leftMargin: Style.space(8); anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter
                  Text { textFormat: Text.PlainText; width: parent.width; text: root.routeUrl(modelData); color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true; elide: Text.ElideRight }
                  Text { textFormat: Text.PlainText; width: parent.width; text: modelData.active ? modelData.target : (modelData.error || "Inactive"); color: modelData.active ? root.secondaryText : Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.caption; elide: Text.ElideRight }
                }
              }
            }
            Text { textFormat: Text.PlainText; anchors.centerIn: parent; visible: routeList.count === 0; text: "No local routes configured"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall }
          }
        }
      }

      Item {
        id: settingsPage
        implicitHeight: Style.space(49) + Math.min(Style.space(525), settingsColumn.implicitHeight)

        Column {
          id: settingsPageColumn
          anchors.fill: parent
          spacing: Style.space(10)
          Row {
            width: parent.width; height: Style.space(28); spacing: Style.space(7)
            PanelActionButton { iconText: "󰁍"; tooltipText: "Back to projects"; focusable: true; onClicked: root.section = "stack" }
            Text { textFormat: Text.PlainText; anchors.verticalCenter: parent.verticalCenter; text: "Settings"; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.title; font.bold: true }
          }
          PanelSeparator { width: parent.width }
          Controls.ScrollView {
            id: settingsScroll
            width: parent.width; height: Math.max(0, Math.min(parent.height-y, settingsColumn.implicitHeight)); clip: true; contentWidth: availableWidth
            Column {
              id: settingsColumn
              width: settingsScroll.availableWidth; spacing: Style.space(10)
              Text { textFormat: Text.PlainText; visible: Boolean(root.service && root.service.settingsPending); text: "Saving settings…"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
              PanelSectionHeader { text: "APPEARANCE" }
              CompactToggleRow {
                width: parent.width
                label: "Bar status counts"
                checked: root.showStatusCounts
                onToggled: root.showStatusCountsRequested(!checked)
              }
              PanelSeparator { width: parent.width }
              PanelSectionHeader { text: "MONITORING" }
              Row {
                width: parent.width
                spacing: Style.space(6)
                NumberField { width: (parent.width - parent.spacing * 2) / 3; fieldWidth: width; label: "Poll (s)"; from: 1; to: 60; value: root.settings && Object.keys(root.settings).length ? root.settings.pollIntervalSeconds : 2; onModified: function(value) { root.mutateSettings("", "pollIntervalSeconds", value) } }
                NumberField { width: (parent.width - parent.spacing * 2) / 3; fieldWidth: width; label: "Samples"; from: 10; to: 600; value: root.settings && Object.keys(root.settings).length ? root.settings.historySamples : 60; onModified: function(value) { root.mutateSettings("", "historySamples", value) } }
                NumberField { width: (parent.width - parent.spacing * 2) / 3; fieldWidth: width; label: "Log lines"; from: 100; to: 50000; stepSize: 100; value: root.settings && Object.keys(root.settings).length ? root.settings.logBufferLines : 2000; onModified: function(value) { root.mutateSettings("", "logBufferLines", value) } }
              }
              PanelSeparator { width: parent.width }
              PanelSectionHeader { text: "NOTIFICATIONS" }
              CompactToggleRow { width: parent.width; label: "Unhealthy"; checked: root.settings && Object.keys(root.settings).length ? root.settings.notifications.unhealthy : true; onToggled: root.mutateSettings("notifications", "unhealthy", !checked) }
              CompactToggleRow { width: parent.width; label: "Crashed"; checked: root.settings && Object.keys(root.settings).length ? root.settings.notifications.crashed : true; onToggled: root.mutateSettings("notifications", "crashed", !checked) }
              CompactToggleRow { width: parent.width; label: "Recovered"; checked: root.settings && Object.keys(root.settings).length ? root.settings.notifications.recovered : true; onToggled: root.mutateSettings("notifications", "recovered", !checked) }
              PanelSeparator { width: parent.width }
              Button {
                width: parent.width
                leftAlign: true
                bordered: true
                focusable: true
                iconText: root.advancedSettingsExpanded ? "󰅀" : "󰅂"
                text: "Advanced & maintenance"
                onClicked: root.advancedSettingsExpanded = !root.advancedSettingsExpanded
              }
              Column {
                visible: root.advancedSettingsExpanded
                width: parent.width
                spacing: Style.space(10)

                PanelSectionHeader { text: "LOCAL PROXY" }
                CompactToggleRow { width: parent.width; label: "HTTP proxy · " + (root.settings && Object.keys(root.settings).length ? root.settings.proxy.listenHost : "127.0.0.1"); checked: root.settings && Object.keys(root.settings).length ? root.settings.proxy.enabled : false; onToggled: root.mutateSettings("proxy", "enabled", !checked) }
                NumberField { width: parent.width; fieldWidth: parent.width; label: "HTTP port"; from: 1; to: 65535; value: root.settings && Object.keys(root.settings).length ? root.settings.proxy.httpPort : 8088; onModified: function(value) { root.mutateSettings("proxy", "httpPort", value) } }

                PanelSectionHeader { text: "COMPOSE IMPORT" }
                Row { width: parent.width; spacing: Style.space(5)
                  TextField { id: composeImportPath; width: parent.width - composeDiscover.width - parent.spacing; placeholderText: "Path to compose.yaml" }
                  Button { id: composeDiscover; iconText: "󰡨"; tooltipText: "Discover Compose services"; enabled: root.selectedProject !== null; focusable: true; onClicked: root.service.importCompose(composeImportPath.text.trim()) }
                }
                Repeater { model: root.service ? root.service.composeImport : []
                  Button { required property var modelData; width: parent.width; leftAlign: true; text: modelData.name; iconText: "󰐕"; focusable: true; onClicked: root.addComposeService(modelData) }
                }

                PanelSectionHeader { text: "MAINTENANCE" }
                Row { width: parent.width; spacing: Style.space(5)
                  Button { text: "Export"; iconText: "󰈇"; focusable: true; onClicked: root.service.request("config.export", {}) }
                  Button { text: "Doctor"; iconText: "󰔚"; focusable: true; onClicked: root.service.request("doctor", {}) }
                  Button { text: "Cleanup"; iconText: "󰃢"; focusable: true; onClicked: root.service.request("cleanup", {}) }
                }
                Row { width: parent.width; spacing: Style.space(5)
                  TextField { id: importPath; width: parent.width - importButton.width - parent.spacing; placeholderText: "Backup path" }
                  Button { id: importButton; text: "Import"; iconText: "󰋺"; focusable: true; enabled: importPath.text.trim() !== ""; onClicked: importDialog.opened = true }
                }
                Repeater { model: root.service && root.service.snapshot.diagnostics ? root.service.snapshot.diagnostics : []
                  Text { textFormat: Text.PlainText; required property var modelData; width: parent.width; text: modelData.code + " · " + modelData.message; color: modelData.level === "error" ? Color.urgent : Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
                }
                TextEdit {
                  width: parent.width; visible: Boolean(root.service && root.service.maintenanceResult)
                  text: root.service ? root.service.maintenanceResult || "" : ""
                  textFormat: TextEdit.PlainText; readOnly: true; selectByMouse: true; wrapMode: TextEdit.WrapAnywhere
                  color: root.primaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption
                }
                Text { textFormat: Text.PlainText; visible: Boolean(root.service && (!root.service.snapshot.diagnostics || root.service.snapshot.diagnostics.length === 0)); text: "󰄬 No diagnostics reported"; color: Color.accent; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall }
              }
              Item { width: 1; height: Style.space(8) }
            }
          }
        }
      }

      Item {
        Column {
          anchors.fill: parent
          spacing: Style.space(9)
          Row {
            width: parent.width; height: Style.space(32); spacing: Style.space(7)
            PanelActionButton { iconText: "󰁍"; tooltipText: "Back to projects"; focusable: true; onClicked: root.section = "stack" }
            Column { width: parent.width - Style.space(32); anchors.verticalCenter: parent.verticalCenter; spacing: Style.space(1)
              Text { textFormat: Text.PlainText; width: parent.width; text: root.service && root.service.logsTarget === root.selectedProjectId && root.selectedProject ? root.selectedProject.name + " logs" : (root.selectedService ? root.selectedService.name + " logs" : "Logs"); color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.title; font.bold: true; elide: Text.ElideRight }
              Text { textFormat: Text.PlainText; text: "journald · visible buffer only"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
            }
          }
          PanelSeparator { width: parent.width }
          LogsView {
            width: parent.width; height: Math.max(0, parent.height-y)
            service: root.service && root.service.logsTarget === root.selectedProjectId ? root.selectedProject : root.selectedService
            entries: root.service ? root.service.logs : []
            loading: root.service ? root.service.logsLoading : false
            notice: root.service ? root.service.logsNotice || "" : ""
            showServiceLabels: Boolean(root.service && root.service.logsTarget === root.selectedProjectId)
            onRefreshRequested: function(query) { if (root.service && root.service.logsTarget) root.service.loadLogs(root.service.logsTarget, query) }
            onClearRequested: root.service.clearVisibleLogs()
            onCloseRequested: root.section = "stack"
          }
        }
      }
    }

    BorderSurface {
      visible: Boolean(root.service && (root.service.lastError !== "" || root.service.actionStatus !== ""))
      anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
      height: toastText.implicitHeight + Style.space(14)
      color: root.service && root.service.lastError !== "" ? Util.alpha(Color.urgent, 0.94) : Util.alpha(Color.popups.background, 0.96)
      borderSpec: Border.flat(root.service && root.service.lastError !== "" ? Color.urgent : Color.accent, 1)
      radius: Style.cornerRadius
      z: 30
      Text { textFormat: Text.PlainText; id: toastText; anchors.fill: parent; anchors.margins: Style.space(7); text: !root.service ? "" : (root.service.lastError !== "" ? root.service.lastError : root.service.actionStatus); color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
    }

    ProjectWizard {
      id: projectWizard; anchors.fill: parent
      onCanceled: opened = false
      onCompleted: function(project) { root.saveEditor(projectWizard, root.service.createProject(project)) }
    }
    ProjectEditor {
      id: projectEditor; anchors.fill: parent
      onCanceled: opened = false
      onSaved: function(project) { root.saveEditor(projectEditor, root.service.updateProject(project)) }
    }
    ServiceEditor {
      id: serviceEditor; anchors.fill: parent; allServices: root.allServices()
      onCanceled: opened = false
      onSaved: function(projectId, serviceData, editing) {
        root.saveEditor(serviceEditor, editing ? root.service.updateService(serviceData) : root.service.createService(projectId, serviceData))
      }
    }
    ConfirmDialog {
      id: deleteDialog; anchors.fill: parent; confirmText: "Delete"
      onCanceled: opened = false
      onConfirmed: {
        opened = false
        if (root.pendingDeleteType === "kill") root.service.forceKill(root.pendingDeleteId)
        else if (root.pendingDeleteType === "project") root.service.deleteProject(root.pendingDeleteId)
        else root.service.deleteService(root.pendingDeleteId)
      }
    }
    ConfirmDialog {
      id: importDialog; anchors.fill: parent
      message: "Replace all definitions and settings with this trusted backup? Import never starts services."
      confirmText: "Replace"
      onCanceled: opened = false
      onConfirmed: { opened = false; root.service.request("config.import", { path: importPath.text.trim(), replace: true }) }
    }
  }

  Connections {
    target: root.service && root.service.requestFinished ? root.service : null
    ignoreUnknownSignals: true
    function onRequestFinished(token, method, success, message) {
      if (token !== root.pendingSaveToken || !root.pendingEditor) return
      root.pendingEditor.saving = false
      root.pendingEditor.saveError = message
      if (success) root.pendingEditor.opened = false
      root.pendingEditor = null
      root.pendingSaveToken = ""
    }
  }

  Process {
    id: openUrlProcess
    running: false
    command: []
    onExited: function(code) { if (code !== 0 && root.service) root.service.lastError = "Could not open the URL" }
  }
}
