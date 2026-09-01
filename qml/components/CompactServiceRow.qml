import QtQuick
import qs.Commons
import qs.Ui
import "."

Button {
  id: root

  required property var serviceData
  property var runtimeData: ({ status: "stopped", cpu: 0, memoryMb: 0, ports: [], history: [] })
  property bool expanded: false
  property bool panelVisible: true
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

  readonly property string state: String(runtimeData.status || "stopped")
  readonly property bool runningState: state === "running" || state === "unhealthy"
  readonly property bool transitioning: state === "starting" || state === "stopping"
  readonly property bool hasError: (state === "crashed" || state === "unhealthy") && String(runtimeData.lastError || "") !== ""
  readonly property bool actionsVisible: pointer.hovered || activeFocus

  function stateTextColor() {
    if (state === "crashed" || state === "unhealthy") return Color.urgent
    if (state === "running" || state === "starting" || state === "stopping") return Color.accent
    return secondaryText
  }

  width: parent ? parent.width : implicitWidth
  height: expanded ? Style.space(82) : (hasError ? Style.space(46) : Style.space(34))
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
  Accessible.name: (root.serviceData.name || "Unnamed service") + ", " + root.state
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
      status: root.state
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
        text: Number(root.runtimeData.memoryMb || 0).toFixed(0) + "MB"
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
        text: root.state.charAt(0).toUpperCase() + root.state.slice(1)
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
        iconText: root.runningState ? "󰓛" : "󰐊"
        tooltipText: root.runningState ? "Stop service" : "Start service"
        enabled: !root.transitioning
        focusable: true
        onClicked: root.runningState ? root.stopRequested(root.serviceData.id) : root.startRequested(root.serviceData.id)
      }
      PanelActionButton { iconText: "󰑓"; tooltipText: "Restart service"; focusable: true; onClicked: root.restartRequested(root.serviceData.id) }
      PanelActionButton { iconText: "󰆍"; tooltipText: "View logs"; focusable: true; onClicked: root.logsRequested(root.serviceData.id) }
      PanelActionButton { iconText: "󰏫"; tooltipText: "Edit service"; focusable: true; onClicked: root.editRequested(root.serviceData.id) }
      PanelActionButton { iconText: "󰆴"; tooltipText: "Delete service"; hoverColor: Color.urgent; focusable: true; onClicked: root.deleteRequested(root.serviceData.id) }
    }

    Text { textFormat: Text.PlainText;
      visible: root.hasError && !root.expanded
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
          Sparkline { width: parent.width; height: Style.space(30); samples: root.runtimeData.history || []; valueKey: "cpu"; lineColor: root.projectColor; updatesEnabled: root.panelVisible }
        }
        Column {
          width: (parent.width - parent.spacing) / 2
          spacing: Style.space(1)
          Row {
            width: parent.width
            Text { textFormat: Text.PlainText; id: memLabel; text: "MEM"; color: root.secondaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
            Item { width: parent.width - memLabel.width - memValue.width; height: 1 }
            Text { textFormat: Text.PlainText; id: memValue; text: Number(root.runtimeData.memoryMb || 0).toFixed(0) + " MB"; color: root.primaryText; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; font.bold: true }
          }
          Sparkline { width: parent.width; height: Style.space(30); samples: root.runtimeData.history || []; valueKey: "memoryMb"; lineColor: root.secondaryText; updatesEnabled: root.panelVisible }
        }
      }
    }
  }
}
