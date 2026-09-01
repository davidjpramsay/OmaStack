import QtQuick
import qs.Commons
import qs.Ui

BorderSurface {
  id: root
  property string iconText: ""
  property string label: ""
  property color foreground: Color.foreground

  implicitWidth: row.implicitWidth + Style.space(12)
  implicitHeight: Style.space(24)
  color: Util.alpha(root.foreground, 0.035)
  borderSpec: Border.flat(Util.alpha(root.foreground, 0.16), Style.normalBorderWidth)
  radius: Style.cornerRadius

  Row {
    id: row
    anchors.centerIn: parent
    spacing: Style.space(4)
    Text { textFormat: Text.PlainText;
      text: root.iconText
      color: Color.accent
      font.family: Style.font.family
      font.pixelSize: Style.font.caption
    }
    Text { textFormat: Text.PlainText;
      text: root.label
      color: root.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.caption
    }
  }
}
