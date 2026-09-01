import QtQuick
import qs.Commons
import qs.Ui

Item {
  id: root
  property bool opened: false
  property var source: null
  signal canceled()
  signal saved(var projectData)

  function begin(projectData) {
    source = projectData
    nameField.text = projectData ? String(projectData.name || "") : ""
    descriptionField.text = projectData ? String(projectData.description || "") : ""
    iconField.text = projectData ? String(projectData.icon || "󰆍") : "󰆍"
    colorField.text = projectData ? String(projectData.color || "") : ""
    opened = true
  }

  visible: opened
  z: 46

  Rectangle {
    anchors.fill: parent
    color: Util.alpha(Color.background, 0.82)
    MouseArea { anchors.fill: parent; onClicked: {} }
    BorderSurface {
      width: Math.min(parent.width - Style.space(48), Style.space(520))
      height: content.implicitHeight + Style.space(40)
      anchors.centerIn: parent
      color: Color.popups.background
      borderSpec: Border.localOrSurfaceSpec("popups", "border", Color.popups.border, Color.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius
      Column {
        id: content
        anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top
        anchors.margins: Style.space(20); spacing: Style.space(10)
        Text { textFormat: Text.PlainText; text: "Project details"; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.heading; font.bold: true }
        TextField { id: nameField; width: parent.width; placeholderText: "Project name"; Accessible.name: "Project name" }
        TextField { id: descriptionField; width: parent.width; placeholderText: "Description"; Accessible.name: "Project description" }
        Row {
          width: parent.width; spacing: Style.space(8)
          TextField { id: iconField; width: Style.space(110); placeholderText: "Icon"; Accessible.name: "Project icon" }
          TextField { id: colorField; width: parent.width - iconField.width - Style.space(8); placeholderText: "Optional colour hint · #RRGGBB"; Accessible.name: "Project color hint" }
        }
        Row {
          width: parent.width; spacing: Style.space(8)
          Button { text: "Cancel"; focusable: true; onClicked: root.canceled() }
          Item { width: parent.width - Style.space(180); height: 1 }
          Button {
            text: "Save"; iconText: "󰄬"; active: true; focusable: true
            onClicked: {
              if (!root.source || nameField.text.trim() === "") { nameField.forceActiveFocus(); return }
              var copy = JSON.parse(JSON.stringify(root.source))
              copy.name = nameField.text.trim(); copy.description = descriptionField.text.trim(); copy.icon = iconField.text.trim(); copy.color = colorField.text.trim()
              root.saved(copy)
            }
          }
        }
      }
    }
  }
}
