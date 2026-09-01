import QtQuick
import QtQuick.Controls as Controls
import QtQuick.Layouts as Layouts
import qs.Commons
import qs.Ui

Item {
  id: root
  property bool opened: false
  property int page: 0
  signal canceled()
  signal completed(var project)

  function reset() {
    page = 0
    projectName.text = ""
    projectDescription.text = ""
    projectIcon.text = "󰆍"
    serviceName.text = ""
    executable.text = ""
    arguments.text = ""
    workingDirectory.text = ""
    autostart.checked = false
    shellMode.checked = false
    shellCommand.text = ""
  }

  function finish() {
    var args = String(arguments.text || "").split("\n").filter(function(value) { return value.length > 0 })
    var service = {
      name: serviceName.text.trim(),
      description: "",
      command: { executable: executable.text.trim(), arguments: args },
      workingDirectory: workingDirectory.text.trim(),
      environment: {},
      autostart: autostart.checked,
      restart: { mode: "never", delaySeconds: 1, maxAttempts: 0, resetAfterSeconds: 60 },
      stopSignal: "SIGTERM",
      gracefulStopSeconds: 10,
      dependencies: [],
      url: "",
      notes: ""
    }
    if (shellMode.checked) service.shell = { enabled: true, shell: "/bin/sh", command: shellCommand.text }
    completed({
      name: projectName.text.trim(),
      description: projectDescription.text.trim(),
      icon: projectIcon.text.trim() || "󰆍",
      color: "",
      services: [service]
    })
  }

  visible: opened
  z: 40

  Rectangle {
    anchors.fill: parent
    color: Util.alpha(Color.background, 0.82)
    MouseArea { anchors.fill: parent; onClicked: {} }

    BorderSurface {
      id: card
      width: Math.min(parent.width - Style.space(48), Style.space(680))
      height: Math.min(parent.height - Style.space(48), Style.space(620))
      anchors.centerIn: parent
      color: Color.popups.background
      borderSpec: Border.localOrSurfaceSpec("popups", "border", Color.popups.border, Color.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius

      Column {
        anchors.fill: parent
        anchors.margins: Style.space(22)
        spacing: Style.space(14)

        Row {
          width: parent.width
          spacing: Style.space(12)
          Column {
            width: parent.width - closeButton.width - Style.space(12)
            Text { textFormat: Text.PlainText;
              text: "Create a project"
              color: Color.foreground
              font.family: Style.font.family
              font.pixelSize: Style.font.heading
              font.bold: true
            }
            Text { textFormat: Text.PlainText;
              text: ["Identity and grouping", "First service command", "Lifecycle behaviour", "Review before saving"][root.page]
              color: Color.muted
              font.family: Style.font.family
              font.pixelSize: Style.font.bodySmall
            }
          }
          Button { id: closeButton; iconText: "󰅖"; tooltipText: "Cancel"; focusable: true; onClicked: root.canceled() }
        }

        Row {
          width: parent.width
          spacing: Style.space(6)
          Repeater {
            model: 4
            Rectangle {
              required property int index
              width: (parent.width - Style.space(18)) / 4
              height: Style.space(3)
              radius: height / 2
              color: index <= root.page ? Color.accent : Util.alpha(Color.foreground, 0.15)
            }
          }
        }

        Layouts.StackLayout {
          id: pages
          width: parent.width
          height: parent.height - navigation.height - Style.space(92)
          currentIndex: root.page

          Controls.ScrollView {
            clip: true
            Column {
              width: pages.width
              spacing: Style.space(14)
              TextField { id: projectName; width: parent.width; placeholderText: "Project name"; Accessible.name: "Project name" }
              TextField { id: projectDescription; width: parent.width; placeholderText: "Optional description"; Accessible.name: "Project description" }
              Row {
                width: parent.width
                spacing: Style.space(10)
                TextField { id: projectIcon; width: Style.space(90); placeholderText: "Icon"; Accessible.name: "Project icon" }
                Text { textFormat: Text.PlainText;
                  width: parent.width - projectIcon.width - Style.space(10)
                  text: "Use a glyph from Omarchy’s active Nerd Font. The interface always inherits the current theme and font."
                  wrapMode: Text.WordWrap
                  color: Color.muted
                  font.family: Style.font.family
                  font.pixelSize: Style.font.bodySmall
                }
              }
            }
          }

          Controls.ScrollView {
            clip: true
            Column {
              width: pages.width
              spacing: Style.space(14)
              TextField { id: serviceName; width: parent.width; placeholderText: "Service name (for example frontend)"; Accessible.name: "Service name" }
              TextField { id: executable; width: parent.width; placeholderText: "Executable path (for example /usr/bin/npm)"; Accessible.name: "Executable path" }
              Column {
                width: parent.width
                spacing: Style.space(5)
                Text { textFormat: Text.PlainText; text: "ARGUMENTS · ONE PER LINE"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true }
                Controls.TextArea {
                  id: arguments
                  width: parent.width
                  height: Style.space(120)
                  color: Color.foreground
                  placeholderText: "run\ndev\n--host\n127.0.0.1"
                  font.family: Style.font.family
                  font.pixelSize: Style.font.body
                  wrapMode: TextEdit.NoWrap
                  Accessible.name: "Command arguments, one per line"
                  background: BorderSurface {
                    color: Style.controlFill(arguments.activeFocus, arguments.hovered, Color.foreground, Color.accent)
                    borderSpec: Border.controlSpec(arguments.activeFocus ? "focus" : "normal", Color.foreground, Color.accent)
                    radius: Style.cornerRadius
                  }
                }
              }
              TextField { id: workingDirectory; width: parent.width; placeholderText: "Working directory (absolute path)"; Accessible.name: "Working directory" }
            }
          }

          Controls.ScrollView {
            clip: true
            Column {
              width: pages.width
              spacing: Style.space(10)
              Toggle {
                id: autostart
                width: parent.width
                label: "Start with OmaStack"
                description: "The user daemon starts this service after login and reconciles it after shell reloads."
                onClicked: checked = !checked
              }
              Toggle {
                id: shellMode
                width: parent.width
                label: "Explicit shell mode"
                description: "Less safe: enables shell syntax and expansion. Prefer executable plus argument fields."
                onClicked: checked = !checked
              }
              TextField {
                id: shellCommand
                visible: shellMode.checked
                width: parent.width
                placeholderText: "Shell command"
                Accessible.name: "Shell command"
              }
              BorderSurface {
                visible: shellMode.checked
                width: parent.width
                height: warningText.implicitHeight + Style.space(20)
                color: Util.alpha(Color.urgent, 0.08)
                borderSpec: Border.flat(Util.alpha(Color.urgent, 0.45), Style.normalBorderWidth)
                radius: Style.cornerRadius
                Text { textFormat: Text.PlainText;
                  id: warningText
                  anchors.fill: parent
                  anchors.margins: Style.space(10)
                  text: "Shell mode executes the command through /bin/sh. Treat project definitions as executable code and never paste untrusted commands here."
                  wrapMode: Text.WordWrap
                  color: Color.foreground
                  font.family: Style.font.family
                  font.pixelSize: Style.font.bodySmall
                }
              }
            }
          }

          Controls.ScrollView {
            clip: true
            Column {
              width: pages.width
              spacing: Style.space(12)
              PanelSectionHeader { text: "PROJECT" }
              Text { textFormat: Text.PlainText; text: projectIcon.text + "  " + (projectName.text || "Unnamed project"); color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.heading; font.bold: true }
              Text { textFormat: Text.PlainText; text: projectDescription.text || "No project description"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.body; wrapMode: Text.WordWrap; width: parent.width }
              PanelSeparator { width: parent.width }
              PanelSectionHeader { text: "FIRST SERVICE" }
              Text { textFormat: Text.PlainText; text: serviceName.text || "Unnamed service"; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.title; font.bold: true }
              Text { textFormat: Text.PlainText;
                text: shellMode.checked ? shellCommand.text : ([executable.text].concat(String(arguments.text || "").split("\n")).join(" "))
                color: Color.muted
                font.family: Style.font.family
                font.pixelSize: Style.font.bodySmall
                wrapMode: Text.WrapAnywhere
                width: parent.width
              }
              Text { textFormat: Text.PlainText; text: autostart.checked ? "󰐊 Autostart enabled" : "󰓛 Manual start"; color: Color.accent; font.family: Style.font.family; font.pixelSize: Style.font.body }
            }
          }
        }

        Row {
          id: navigation
          width: parent.width
          spacing: Style.space(8)
          Button { visible: root.page > 0; text: "Back"; iconText: "󰁍"; focusable: true; onClicked: root.page-- }
          Item { width: parent.width - previousWidth - nextButton.width - Style.space(8); height: 1; property real previousWidth: root.page > 0 ? Style.space(84) : 0 }
          Button {
            id: nextButton
            text: root.page === 3 ? "Create project" : "Continue"
            iconText: root.page === 3 ? "󰐕" : "󰁔"
            active: true
            focusable: true
            onClicked: {
              if (root.page === 0 && projectName.text.trim() === "") { projectName.forceActiveFocus(); return }
              if (root.page === 1 && serviceName.text.trim() === "") { serviceName.forceActiveFocus(); return }
              if (root.page < 3) root.page++
              else {
                if (!shellMode.checked && executable.text.trim() === "") { root.page = 1; executable.forceActiveFocus(); return }
                if (workingDirectory.text.trim() === "") { root.page = 1; workingDirectory.forceActiveFocus(); return }
                root.finish()
              }
            }
          }
        }
      }
    }
  }
}
