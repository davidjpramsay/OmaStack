import QtQuick
import QtQuick.Controls as Controls
import qs.Commons
import qs.Ui
import "."

Button {
  id: root

  required property var serviceData
  property var runtimeData: ({ status: "stopped", cpu: 0, memoryMb: 0, ports: [], history: [] })
  property bool expanded: false
  property bool panelVisible: true
  property int historySamples: 60
  property string routeUrl: ""
  property color projectColor: Color.accent
  property color primaryText: Color.popups.text
  property color secondaryText: Qt.tint(Color.popups.background, Util.alpha(primaryText, 0.82))

  signal startRequested(string serviceId)
  signal stopRequested(string serviceId)
  signal restartRequested(string serviceId)
  signal openRequested(string url)
  signal logsRequested(string serviceId)
  signal editRequested(string serviceId)
  signal deleteRequested(string serviceId)
  signal dockerActionRequested(string serviceId, string action)
  signal dockerTerminalRequested(string serviceId)
  signal forceKillRequested(string serviceId)

  readonly property string serviceState: String(runtimeData.status || "stopped")
  readonly property bool runningState: serviceState === "running" || serviceState === "unhealthy"
  readonly property bool transitioning: serviceState === "starting" || serviceState === "stopping"
  readonly property bool hasError: (serviceState === "crashed" || serviceState === "unhealthy") && String(runtimeData.lastError || "") !== ""
  readonly property bool actionsVisible: true
  readonly property bool canStop: runningState || serviceState === "starting"
  readonly property string browserUrl: {
    var configured = String(serviceData.url || "").trim()
    if (configured !== "") return configured
    return routeUrl
  }

  function openPort(port) {
    port = Number(port)
    if (isFinite(port) && Math.floor(port) === port && port > 0 && port <= 65535) root.openRequested("http://127.0.0.1:" + String(port))
  }

  function stateTextColor() {
    if (serviceState === "crashed" || serviceState === "unhealthy") return Color.urgent
    if (serviceState === "running" || serviceState === "starting" || serviceState === "stopping") return Color.accent
    return secondaryText
  }

  width: parent ? parent.width : implicitWidth
  height: Style.space((expanded ? 82 : 34) + (hasError ? 18 : 0))
  leftAlign: true
  focusable: true
  selected: false
  bordered: expanded
  foreground: primaryText
  background: expanded ? Style.selectedFillFor(primaryText, Color.accent, Color.urgent) : "transparent"
  horizontalPadding: 0
  verticalPadding: 0
  text: ""
  iconText: ""
  Accessible.name: (root.serviceData.name || "Unnamed service") + ", " + root.serviceState
  Accessible.description: "Open service metrics and controls"
  onRightClicked: root.editRequested(root.serviceData.id)

  Behavior on height { NumberAnimation { duration: 100; easing.type: Easing.OutCubic } }

  HoverHandler { id: pointer }

  Item {
    anchors.fill: parent
    anchors.leftMargin: Style.space(8)
    anchors.rightMargin: Style.space(6)

    StatusDot {
      id: stateDot
      anchors.left: parent.left
      anchors.top: parent.top
      anchors.topMargin: Style.space(14)
      status: root.serviceState
      dotSize: Style.space(5)
    }

    Text { textFormat: Text.PlainText;
      id: serviceName
      anchors.left: stateDot.right
      anchors.leftMargin: Style.space(7)
      anchors.right: root.actionsVisible ? actions.left : metrics.left
      anchors.rightMargin: Style.space(8)
      anchors.top: parent.top
      anchors.topMargin: Style.space(8)
      text: root.serviceData.name || "Unnamed"
      color: root.primaryText
      font.family: Style.font.family
      font.pixelSize: Style.font.bodySmall
      font.bold: true
      elide: Text.ElideRight
    }

    Row {
      id: metrics
      visible: !root.actionsVisible
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.topMargin: Style.space(8)
      spacing: Style.space(5)

      Text { textFormat: Text.PlainText;
        visible: root.runningState
        text: Number(root.runtimeData.cpu || 0).toFixed(1) + "%"
        color: root.secondaryText
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
      Text { textFormat: Text.PlainText; visible: root.runningState; text: "·"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.caption }
      Text { textFormat: Text.PlainText;
        visible: root.runningState
        text: Number(root.runtimeData.memoryMb || 0).toFixed(0) + "MiB"
        color: root.secondaryText
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
      Text { textFormat: Text.PlainText;
        visible: Boolean(root.runtimeData.ports && root.runtimeData.ports.length > 0)
        text: ":" + String(root.runtimeData.ports && root.runtimeData.ports.length ? root.runtimeData.ports[0] : "")
        color: Color.accent
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
      Text { textFormat: Text.PlainText;
        visible: !root.runningState
        text: root.serviceState.charAt(0).toUpperCase() + root.serviceState.slice(1)
        color: root.stateTextColor()
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
    }

    Row {
      id: actions
      visible: root.actionsVisible
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.topMargin: Style.space(3)
      spacing: Style.space(2)

      PanelActionButton {
        iconText: root.canStop ? "󰓛" : "󰐊"
        tooltipText: root.serviceState === "starting" ? "Cancel startup / stop service" : (root.canStop ? "Stop service" : "Start service")
        enabled: root.serviceState !== "stopping"
        focusable: true
        onClicked: root.canStop ? root.stopRequested(root.serviceData.id) : root.startRequested(root.serviceData.id)
      }
      PanelActionButton {
        id: browserButton
        iconText: "󰖟"
        tooltipText: root.browserUrl !== "" ? "Open service in browser" : "Choose a port or set a browser URL"
        enabled: root.runningState && !root.transitioning
        focusable: true
        onClicked: root.browserUrl !== "" ? root.openRequested(root.browserUrl) : portMenu.open()
      }
      PanelActionButton { id: moreButton; iconText: "󰇙"; tooltipText: "More service actions"; focusable: true; onClicked: actionsMenu.open() }
    }

    Text { textFormat: Text.PlainText;
      visible: root.hasError
      anchors.left: serviceName.left
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.topMargin: Style.space(26)
      text: String(root.runtimeData.lastError || "")
      color: Color.urgent
      font.family: Style.font.family
      font.pixelSize: Style.font.caption
      elide: Text.ElideRight
    }

    Item {
      visible: root.expanded
      anchors.left: serviceName.left
      anchors.right: parent.right
      anchors.bottom: parent.bottom
      height: Style.space(45)

      Row {
        anchors.fill: parent
        spacing: Style.space(6)

        Column {
          width: (parent.width - parent.spacing) / 2
          spacing: Style.space(1)
          Row {
            width: parent.width
            Text { textFormat: Text.PlainText; id: cpuLabel; text: "CPU"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
            Item { width: parent.width - cpuLabel.width - cpuValue.width; height: 1 }
            Text { textFormat: Text.PlainText; id: cpuValue; text: Number(root.runtimeData.cpu || 0).toFixed(1) + "%"; color: root.primaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
          }
          Sparkline { width: parent.width; height: Style.space(30); samples: root.runtimeData.history || []; maxSamples: root.historySamples; valueKey: "cpu"; lineColor: root.projectColor; updatesEnabled: root.panelVisible }
        }
        Column {
          width: (parent.width - parent.spacing) / 2
          spacing: Style.space(1)
          Row {
            width: parent.width
            Text { textFormat: Text.PlainText; id: memLabel; text: "MEM"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
            Item { width: parent.width - memLabel.width - memValue.width; height: 1 }
            Text { textFormat: Text.PlainText; id: memValue; text: Number(root.runtimeData.memoryMb || 0).toFixed(0) + " MiB"; color: root.primaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
          }
          Sparkline { width: parent.width; height: Style.space(30); samples: root.runtimeData.history || []; maxSamples: root.historySamples; valueKey: "memoryMb"; lineColor: root.secondaryText; updatesEnabled: root.panelVisible }
        }
      }
    }
  }

  Controls.Popup {
    id: actionsMenu
    parent: root
    x: Math.max(0, root.width-width); y: Style.space(30)
    width: Style.space(220)
    padding: Style.space(5)
    modal: false; focus: true
    background: BorderSurface { color: Color.popups.background; borderSpec: Border.flat(Color.popups.border, Style.normalBorderWidth); radius: Style.cornerRadius }
    contentItem: Column {
      spacing: Style.space(2)
      Repeater {
        model: [
          {label:"Restart", action:"restart"}, {label:"View logs", action:"logs"}, {label:"Edit service", action:"edit"},
          {label:"Force kill…", action:"kill"}, {label:"Delete service…", action:"delete"}
        ].concat(root.serviceData.docker ? [{label:"Rebuild container",action:"rebuild"},{label:"Recreate container",action:"recreate"},{label:"Container terminal",action:"terminal"}] : [])
        Button {
          required property var modelData
          width: parent.width; text: modelData.label; leftAlign: true; focusable: true
          onClicked: {
            actionsMenu.close()
            var id = root.serviceData.id
            switch (modelData.action) {
              case "restart": root.restartRequested(id); break
              case "logs": root.logsRequested(id); break
              case "edit": root.editRequested(id); break
              case "kill": root.forceKillRequested(id); break
              case "delete": root.deleteRequested(id); break
              case "terminal": root.dockerTerminalRequested(id); break
              default: root.dockerActionRequested(id, modelData.action)
            }
          }
        }
      }
    }
  }

  Controls.Popup {
    id: portMenu
    parent: root
    x: Math.max(0, root.width-width); y: Style.space(30)
    width: Style.space(250); padding: Style.space(7); focus: true
    background: BorderSurface { color: Color.popups.background; borderSpec: Border.flat(Color.popups.border, Style.normalBorderWidth); radius: Style.cornerRadius }
    contentItem: Column {
      spacing: Style.space(4)
      Text { textFormat: Text.PlainText; width: parent.width; text: "Detected TCP ports may not serve HTTP. Set a URL for one-click access."; wrapMode: Text.WordWrap; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
      Repeater {
        model: root.runtimeData.ports || []
        Button { required property var modelData; width: parent.width; text: "Open port " + modelData + " as HTTP"; focusable: true; onClicked: { portMenu.close(); root.openPort(modelData) } }
      }
      Button { width: parent.width; text: "Set browser URL…"; focusable: true; onClicked: { portMenu.close(); root.editRequested(root.serviceData.id) } }
    }
  }
}
