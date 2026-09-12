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
  property int historySamples: 60
  property color projectColor: Color.accent
  signal startRequested(string serviceId)
  signal stopRequested(string serviceId)
  signal restartRequested(string serviceId)
  signal openRequested(string url)
  signal logsRequested(string serviceId)
  signal dockerActionRequested(string serviceId, string action)
  signal dockerTerminalRequested(string serviceId)

  width: parent ? parent.width : implicitWidth
  height: expanded ? Style.space(128) : Style.space(64)
  leftAlign: true
  focusable: true
  selected: expanded
  horizontalPadding: Style.space(10)
  verticalPadding: 0
  text: ""
  iconText: ""

  Behavior on height { NumberAnimation { duration: 130; easing.type: Easing.OutCubic } }

  Item {
    anchors.fill: parent
    anchors.margins: Style.space(10)

    StatusDot {
      id: statusDot
      anchors.left: parent.left
      anchors.top: parent.top
      anchors.topMargin: Style.space(6)
      status: String(root.runtimeData.status || "stopped")
    }

    Column {
      id: identity
      anchors.left: statusDot.right
      anchors.leftMargin: Style.space(8)
      anchors.top: parent.top
      anchors.right: metrics.left
      anchors.rightMargin: Style.space(12)
      spacing: Style.space(2)
      Text { textFormat: Text.PlainText;
        text: root.serviceData.name || "Unnamed service"
        elide: Text.ElideRight
        width: parent.width
        color: Color.foreground
        font.family: Style.font.family
        font.pixelSize: Style.font.body
        font.bold: true
      }
      Text { textFormat: Text.PlainText;
        text: ((root.runtimeData.status === "crashed" || root.runtimeData.status === "unhealthy") && root.runtimeData.lastError)
          ? root.runtimeData.lastError
          : (root.serviceData.description || (root.serviceData.command ? root.serviceData.command.executable : "") || "Not configured")
        elide: Text.ElideMiddle
        width: parent.width
        color: Color.muted
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
    }

    Row {
      id: metrics
      anchors.right: actionButtons.left
      anchors.rightMargin: Style.space(10)
      anchors.top: parent.top
      spacing: Style.space(5)
      MetricPill { iconText: "󰍛"; label: Number(root.runtimeData.cpu || 0).toFixed(1) + "%" }
      MetricPill { iconText: "󰘚"; label: Number(root.runtimeData.memoryMb || 0).toFixed(0) + " MiB" }
      MetricPill {
        visible: Boolean(root.runtimeData.ports && root.runtimeData.ports.length > 0)
        iconText: "󰘖"
        label: root.runtimeData.ports && root.runtimeData.ports.length > 0 ? String(root.runtimeData.ports[0]) : "—"
      }
      MetricPill {
        visible: Boolean(root.runtimeData.health || root.runtimeData.containerHealth)
        iconText: (root.runtimeData.health === "unhealthy" || root.runtimeData.containerHealth === "unhealthy") ? "󰀦" : "󰄬"
        label: root.runtimeData.containerHealth || root.runtimeData.health || ""
        foreground: (root.runtimeData.health === "unhealthy" || root.runtimeData.containerHealth === "unhealthy") ? Color.urgent : Color.foreground
      }
    }

    Row {
      id: actionButtons
      anchors.right: parent.right
      anchors.top: parent.top
      spacing: Style.space(3)
      Button {
        iconText: root.runtimeData.status === "running" || root.runtimeData.status === "unhealthy" ? "󰓛" : "󰐊"
        tooltipText: root.runtimeData.status === "running" || root.runtimeData.status === "unhealthy" ? "Stop service" : "Start service"
        focusable: true
        enabled: root.runtimeData.status !== "starting" && root.runtimeData.status !== "stopping"
        horizontalPadding: Style.space(7)
        verticalPadding: Style.space(5)
        onClicked: {
          if (root.runtimeData.status === "running" || root.runtimeData.status === "unhealthy") root.stopRequested(root.serviceData.id)
          else root.startRequested(root.serviceData.id)
        }
      }
      Button {
        iconText: "󰑓"
        tooltipText: "Restart service"
        focusable: true
        horizontalPadding: Style.space(7)
        verticalPadding: Style.space(5)
        onClicked: root.restartRequested(root.serviceData.id)
      }
      Button {
        iconText: "󰆍"
        tooltipText: "View logs"
        focusable: true
        horizontalPadding: Style.space(7)
        verticalPadding: Style.space(5)
        onClicked: root.logsRequested(root.serviceData.id)
      }
    }

    Item {
      visible: root.expanded
      anchors.left: identity.left
      anchors.right: parent.right
      anchors.bottom: parent.bottom
      height: Style.space(50)
      opacity: root.expanded ? 1 : 0
      readonly property real chartWidth: Math.max(90, parent.width * (root.serviceData.docker ? 0.27 : 0.4))
      Behavior on opacity { NumberAnimation { duration: 100 } }

      Sparkline {
        anchors.left: parent.left
        anchors.top: parent.top
        width: parent.chartWidth
        height: parent.height
        samples: root.runtimeData.history || []
        maxSamples: root.historySamples
        valueKey: "cpu"
        lineColor: root.projectColor
        updatesEnabled: root.panelVisible
      }
      Sparkline {
        anchors.left: parent.left
        anchors.leftMargin: parent.chartWidth + Style.space(8)
        anchors.top: parent.top
        width: parent.chartWidth
        height: parent.height
        samples: root.runtimeData.history || []
        maxSamples: root.historySamples
        valueKey: "memoryMb"
        lineColor: Color.muted
        updatesEnabled: root.panelVisible
      }
      Row {
        visible: Boolean(root.serviceData.docker)
        anchors.right: openButton.visible ? openButton.left : parent.right
        anchors.rightMargin: openButton.visible ? Style.space(4) : 0
        anchors.bottom: parent.bottom
        spacing: Style.space(3)
        Button { iconText: "󰜎"; text: "Build"; tooltipText: "Rebuild Compose image"; focusable: true; onClicked: root.dockerActionRequested(root.serviceData.id, "rebuild") }
        Button { iconText: "󰑓"; text: "Recreate"; tooltipText: "Force-recreate container"; focusable: true; onClicked: root.dockerActionRequested(root.serviceData.id, "recreate") }
        Button { iconText: "󰆍"; text: "Terminal"; tooltipText: "Open interactive container shell"; focusable: true; onClicked: root.dockerTerminalRequested(root.serviceData.id) }
      }
      Button {
        id: openButton
        visible: Boolean(root.serviceData.url || (root.runtimeData.ports && root.runtimeData.ports.length > 0))
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        iconText: "󰖟"
        text: "Open"
        focusable: true
        onClicked: root.openRequested(root.serviceData.url || ("http://127.0.0.1:" + root.runtimeData.ports[0]))
      }
    }
  }
}
