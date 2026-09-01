import QtQuick
import qs.Commons

Rectangle {
  id: root
  property string status: "stopped"
  property real dotSize: Style.space(6)
  readonly property color statusColor: {
    if (status === "running") return Color.accent
    if (status === "unhealthy") return Qt.tint(Color.urgent, Util.alpha(Color.accent, 0.44))
    if (status === "crashed") return Color.urgent
    if (status === "starting" || status === "stopping") return Color.accent
    return Qt.tint(Color.popups.background, Util.alpha(Color.popups.text, 0.72))
  }
  width: dotSize
  height: dotSize
  radius: dotSize / 2
  color: statusColor

  SequentialAnimation on opacity {
    running: root.visible && (root.status === "starting" || root.status === "stopping")
    loops: Animation.Infinite
    NumberAnimation { to: 0.35; duration: 450; easing.type: Easing.InOutSine }
    NumberAnimation { to: 1; duration: 450; easing.type: Easing.InOutSine }
  }
}
