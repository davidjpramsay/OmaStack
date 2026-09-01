import QtQuick
import qs.Commons
import qs.Ui

Button {
  id: root

  property string label: ""
  property bool checked: false
  signal toggled()

  width: parent ? parent.width : implicitWidth
  height: Style.space(32)
  leftAlign: true
  focusable: true
  foreground: Color.foreground
  horizontalPadding: Style.space(7)
  verticalPadding: 0
  text: ""
  iconText: ""
  Accessible.name: label
  Accessible.description: checked ? "On" : "Off"
  onClicked: root.toggled()

  Text { textFormat: Text.PlainText;
    anchors.left: parent.left
    anchors.leftMargin: Style.space(7)
    anchors.right: toggle.left
    anchors.rightMargin: Style.space(7)
    anchors.verticalCenter: parent.verticalCenter
    text: root.label
    color: root.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.bodySmall
    elide: Text.ElideRight
  }

  ToggleSwitch {
    id: toggle
    anchors.right: parent.right
    anchors.rightMargin: Style.space(5)
    anchors.verticalCenter: parent.verticalCenter
    checked: root.checked
    interactive: false
    cursorRing: false
    trackHeight: Style.space(16)
  }
}
