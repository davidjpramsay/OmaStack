import QtQuick
import QtQuick.Controls as Controls
import qs.Commons
import qs.Ui

Item {
  id: root
  property var service: null
  property var entries: []
  property bool loading: false
  property bool paused: false
  property string query: ""
  property color foreground: Color.foreground
  signal refreshRequested(string query)
  signal clearRequested()
  signal closeRequested()

  function visibleEntries() {
    if (!root.query) return root.entries || []
    var needle = root.query.toLowerCase()
    return (root.entries || []).filter(function(entry) {
      return String(entry.message || "").toLowerCase().indexOf(needle) >= 0
        || String(entry.stream || "").toLowerCase().indexOf(needle) >= 0
    })
  }

  Column {
    anchors.fill: parent
    spacing: Style.space(8)

    Row {
      width: parent.width
      spacing: Style.space(6)
      TextField {
        id: search
        width: parent.width - pauseButton.width - refreshButton.width - clearButton.width - closeButton.width - Style.space(24)
        placeholderText: "Filter visible logs"
        Accessible.name: "Filter service logs"
        onTextChanged: root.query = text
        Keys.onEscapePressed: root.closeRequested()
      }
      Button {
        id: pauseButton
        iconText: root.paused ? "󰐊" : "󰏤"
        tooltipText: root.paused ? "Resume following" : "Pause following"
        active: root.paused
        focusable: true
        onClicked: root.paused = !root.paused
      }
      Button {
        id: refreshButton
        iconText: "󰑐"
        tooltipText: "Reload logs"
        focusable: true
        onClicked: root.refreshRequested(root.query)
      }
      Button {
        id: clearButton
        iconText: "󰃢"
        tooltipText: "Clear visible buffer (journald is preserved)"
        focusable: true
        onClicked: root.clearRequested()
      }
      Button {
        id: closeButton
        iconText: "󰅖"
        tooltipText: "Close log viewer"
        focusable: true
        onClicked: root.closeRequested()
      }
    }

    BorderSurface {
      width: parent.width
      height: parent.height - search.height - Style.space(8)
      color: Util.alpha(Color.background, 0.38)
      borderSpec: Border.flat(Util.alpha(root.foreground, 0.16), Style.normalBorderWidth)
      radius: Style.cornerRadius
      clip: true

      ListView {
        id: logList
        anchors.fill: parent
        anchors.margins: Style.space(8)
        model: root.visibleEntries()
        spacing: Style.space(2)
        clip: true
        onCountChanged: if (!root.paused && count > 0) positionViewAtEnd()

        delegate: Row {
          required property var modelData
          width: logList.width
          spacing: Style.space(7)
          Text { textFormat: Text.PlainText;
            text: String(modelData.timestamp || "").substring(11, 23)
            color: Color.muted
            font.family: Style.font.family
            font.pixelSize: Style.font.caption
          }
          Text { textFormat: Text.PlainText;
            text: modelData.stream === "stderr" ? "ERR" : "OUT"
            color: modelData.stream === "stderr" ? Color.urgent : Color.accent
            font.family: Style.font.family
            font.pixelSize: Style.font.caption
            font.bold: true
          }
          Text { textFormat: Text.PlainText;
            visible: Boolean(modelData.service)
            text: modelData.service || ""
            color: Color.muted
            font.family: Style.font.family
            font.pixelSize: Style.font.caption
          }
          TextEdit {
            width: parent.width - x
            textFormat: TextEdit.PlainText
            text: modelData.message || ""
            color: root.foreground
            font.family: Style.font.family
            font.pixelSize: Style.font.caption
            readOnly: true
            selectByMouse: true
            wrapMode: TextEdit.WrapAnywhere
          }
        }

        Text { textFormat: Text.PlainText;
          anchors.centerIn: parent
          visible: logList.count === 0
          text: root.loading ? "Loading logs…" : (root.service ? "No matching journal entries" : "Choose a service to inspect its logs")
          color: Color.muted
          font.family: Style.font.family
          font.pixelSize: Style.font.body
        }
      }
    }
  }

  Timer {
    interval: 2000
    repeat: true
    running: root.visible && !root.paused && root.service !== null
    onTriggered: root.refreshRequested(root.query)
  }
}
