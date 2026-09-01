import QtQuick
import Quickshell.Io
import qs.Commons
import qs.Ui as Ui

// OmaStack follows the same ownership model as Omarchy's first-party panel
// widgets: the bar item owns its layer-shell popup. This keeps the panel
// anchored to the widget instead of exposing it as a tiled application window.
Ui.Panel {
  id: root

  moduleName: "david.omastack"
  ipcTarget: "david.omastack.panel"

  readonly property var stack: bar && bar.shell ? bar.shell.serviceFor(moduleName) : null
  readonly property int runningCount: stack ? stack.runningCount : 0
  readonly property int stoppedCount: stack ? stack.stoppedCount : 0
  readonly property int unhealthyCount: stack ? stack.unhealthyCount : 0
  readonly property int crashedCount: stack ? stack.crashedCount : 0
  readonly property bool hasAttention: unhealthyCount > 0 || crashedCount > 0
  readonly property bool showStatusCounts: booleanSetting("showStatusCounts", !booleanSetting("compact", false))
  readonly property color foreground: bar ? bar.barForeground : Color.foreground
  readonly property color urgent: bar ? bar.urgent : Color.urgent

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  function booleanSetting(name, fallback) {
    var value = setting(name, undefined)
    if (value === undefined || value === null || value === "") return fallback
    if (value === true || value === false) return value
    var normalized = String(value).trim().toLowerCase()
    if (normalized === "true" || normalized === "1" || normalized === "yes" || normalized === "on") return true
    if (normalized === "false" || normalized === "0" || normalized === "no" || normalized === "off") return false
    return fallback
  }

  function open() {
    panelContent.prepareOpen("")
    controller.show()
  }

  function close() {
    panelContent.prepareClose()
    controller.hide()
  }

  function toggle() {
    if (opened) close()
    else open()
  }

  function setShowStatusCounts(value) {
    if (statusCountsProcess.running) return
    statusCountsProcess.command = ["omarchy", "bar", "set", moduleName, "showStatusCounts", value ? "true" : "false", "--json"]
    statusCountsProcess.running = true
  }

  // A shell/plugin reload must never restore an old visible popup. The panel
  // opens only from an explicit bar press or IPC request.
  Component.onCompleted: controller.hide()

  Ui.WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    labelVisible: false
    hasVisualContent: true
    active: root.hasAttention
    tooltipText: root.stack && root.stack.connected
      ? (root.runningCount + " running · " + root.stoppedCount + " stopped · " + root.unhealthyCount + " unhealthy · " + root.crashedCount + " crashed")
      : "OmaStack backend disconnected"
    fixedWidth: statusRow.implicitWidth + Style.space(16)
    onPressed: function(buttonCode) {
      if (buttonCode === Qt.RightButton && root.stack) root.stack.refresh()
      else root.toggle()
    }

    Row {
      id: statusRow
      anchors.centerIn: parent
      spacing: Style.space(5)

      Text { textFormat: Text.PlainText;
        anchors.verticalCenter: parent.verticalCenter
        text: "󰆍"
        color: root.hasAttention ? root.urgent : root.foreground
        font.family: root.bar ? root.bar.fontFamily : Style.font.family
        font.pixelSize: Style.font.icon
      }

      Row {
        visible: root.showStatusCounts
        anchors.verticalCenter: parent.verticalCenter
        spacing: Style.space(4)

        Repeater {
          model: [
            { count: root.runningCount, color: Color.accent },
            { count: root.stoppedCount, color: Color.muted },
            { count: root.unhealthyCount, color: Qt.tint(Color.urgent, Util.alpha(Color.accent, 0.44)) },
            { count: root.crashedCount, color: root.urgent }
          ]

          Row {
            required property var modelData
            visible: modelData.count >= 0
            spacing: Style.space(2)
            Rectangle {
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(4)
              height: width
              radius: width / 2
              color: modelData.color
            }
            Text { textFormat: Text.PlainText;
              anchors.verticalCenter: parent.verticalCenter
              text: modelData.count
              color: root.foreground
              font.family: root.bar ? root.bar.fontFamily : Style.font.family
              font.pixelSize: Style.font.caption
            }
          }
        }
      }
    }
  }

  Ui.KeyboardPanel {
    id: popup
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: panelContent.focusTarget
    contentWidth: popup.fittedContentWidth(Style.space(panelContent.preferredWidth), Style.space(680))
    contentHeight: popup.fittedContentHeight(panelContent.implicitHeight, Style.space(660))

    CompactPanel {
      id: panelContent
      anchors.fill: parent
      service: root.stack
      panelVisible: root.opened
      showStatusCounts: root.showStatusCounts
      onCloseRequested: root.close()
      onSwitchRequested: function(direction) { root.switchPanel(direction) }
      onShowStatusCountsRequested: function(value) { root.setShowStatusCounts(value) }
    }
  }

  Process {
    id: statusCountsProcess
    running: false
    command: []
    onExited: function(exitCode) {
      if (exitCode !== 0 && root.stack) root.stack.lastError = "Could not update the OmaStack bar counters"
    }
  }
}
