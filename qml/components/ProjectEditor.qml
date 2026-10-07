import QtQuick
import QtQuick.Controls as Controls
import qs.Commons
import qs.Ui

FocusScope {
  id: root
  property bool opened: false
  property var source: null
  property bool saving: false
  property string saveError: ""
  property string initialDraft: ""
  signal canceled()
  signal saved(var projectData)

  function draft() {
    var copy = source ? { id: source.id } : {}
    copy.name = nameField.text.trim(); copy.description = descriptionField.text.trim(); copy.icon = iconField.text.trim(); copy.color = colorField.text.trim()
    return copy
  }
  function cancelEditor() {
    if (saving) return
    if (JSON.stringify(draft()) !== initialDraft) discardDialog.opened = true
    else canceled()
  }
  Keys.onEscapePressed: function(event) { if (discardDialog.opened) discardDialog.opened = false; else cancelEditor(); event.accepted = true }
  Keys.onPressed: function(event) { if (discardDialog.opened && discardDialog.handleKey(event)) event.accepted = true }

  function begin(projectData) {
    source = projectData
    saving = false; saveError = ""
    nameField.text = projectData ? String(projectData.name || "") : ""
    descriptionField.text = projectData ? String(projectData.description || "") : ""
    iconField.text = projectData ? String(projectData.icon || "󰆍") : "󰆍"
    colorField.text = projectData ? String(projectData.color || "") : ""
    opened = true
    initialDraft = JSON.stringify(draft())
    Qt.callLater(function() { nameField.forceActiveFocus() })
  }

  visible: opened
  z: 46

  Rectangle {
    anchors.fill: parent
    color: Util.alpha(Color.background, 0.82)
    MouseArea { anchors.fill: parent; onClicked: {} }
    BorderSurface {
      width: Math.min(parent.width - Style.space(48), Style.space(520))
      height: Math.min(parent.height-Style.space(24), content.implicitHeight + Style.space(40))
      anchors.centerIn: parent
      color: Color.popups.background
      borderSpec: Border.localOrSurfaceSpec("popups", "border", Color.popups.border, Color.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius
      Controls.ScrollView {
        anchors.fill: parent; anchors.margins: Style.space(12); clip: true; contentWidth: availableWidth
      Column {
        id: content
        enabled: !root.saving
        width: parent.width
        spacing: Style.space(10)
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
          Button { text: "Cancel"; focusable: true; enabled: !root.saving; onClicked: root.cancelEditor() }
          Item { width: parent.width - Style.space(180); height: 1 }
          Button {
            text: "Save"; iconText: "󰄬"; active: true; focusable: true; enabled: !root.saving
            onClicked: {
              if (!root.source || nameField.text.trim() === "") { nameField.forceActiveFocus(); return }
              root.saved(root.draft())
            }
          }
        }
        Text { textFormat: Text.PlainText; width: parent.width; text: root.saving ? "Saving…" : root.saveError; color: Color.urgent; wrapMode: Text.WordWrap; font.family: Style.font.family; font.pixelSize: Style.font.caption }
      }
      }
    }
  }
  ConfirmDialog { id: discardDialog; anchors.fill: parent; message: "Discard unsaved project changes?"; confirmText: "Discard"; onOpenedChanged: if (opened) root.forceActiveFocus(); onCanceled: opened = false; onConfirmed: { opened = false; root.canceled() } }
}
