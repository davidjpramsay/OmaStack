import QtQuick
import QtQuick.Controls as Controls
import QtQuick.Layouts as Layouts
import qs.Commons
import qs.Ui

FocusScope {
  id: root
  property bool opened: false
  property bool editing: false
  property string projectId: ""
  property var source: null
  property var allServices: []
  property int page: 0
  property bool saving: false
  property string saveError: ""
  property string initialDraft: ""
  signal canceled()
  signal saved(string projectId, var serviceData, bool editing)

  ListModel { id: environmentRows; objectName: "serviceEnvironmentRows" }

  function clone(value) { return JSON.parse(JSON.stringify(value)) }
  function numberOr(value, fallback) { return value === undefined || value === null ? fallback : Number(value) }
  function cancelEditor() {
    if (saving) return
    if (JSON.stringify(buildService()) !== initialDraft) discardDialog.opened = true
    else canceled()
  }

  Keys.onEscapePressed: function(event) {
    if (discardDialog.opened) discardDialog.opened = false
    else cancelEditor()
    event.accepted = true
  }
  Keys.onPressed: function(event) { if (discardDialog.opened && discardDialog.handleKey(event)) event.accepted = true }

  function begin(project, serviceData) {
    projectId = project || ""
    source = serviceData ? clone(serviceData) : null
    saving = false
    saveError = ""
    editing = Boolean(serviceData && serviceData.id)
    page = 0
    environmentRows.clear()
    nameField.text = serviceData ? String(serviceData.name || "") : ""
    descriptionField.text = serviceData ? String(serviceData.description || "") : ""
    executableField.text = serviceData && serviceData.command ? String(serviceData.command.executable || "") : ""
    argumentsField.text = serviceData && serviceData.command && serviceData.command.arguments ? serviceData.command.arguments.join("\n") : ""
    directoryField.text = serviceData ? String(serviceData.workingDirectory || "") : ""
    envFileField.text = serviceData ? String(serviceData.environmentFile || "") : ""
    shellToggle.checked = Boolean(serviceData && serviceData.shell && serviceData.shell.enabled)
    shellCommandField.text = serviceData && serviceData.shell ? String(serviceData.shell.command || "") : ""
    shellPathField.text = serviceData && serviceData.shell ? String(serviceData.shell.shell || "/bin/sh") : "/bin/sh"
    var environment = serviceData && serviceData.environment ? serviceData.environment : {}
    for (var key in environment) environmentRows.append({ keyText: key, valueText: environment[key].keepFrom ? "" : String(environment[key].value || ""), secretValue: environment[key].secret === true, keepFrom: String(environment[key].keepFrom || "") })
    autostartToggle.checked = Boolean(serviceData && serviceData.autostart)
    restartMode.value = serviceData && serviceData.restart ? String(serviceData.restart.mode || "never") : "never"
    restartDelay.value = serviceData && serviceData.restart ? numberOr(serviceData.restart.delaySeconds, 1) : 1
    restartAttempts.value = serviceData && serviceData.restart ? Number(serviceData.restart.maxAttempts || 0) : 0
    stopSignal.value = serviceData ? String(serviceData.stopSignal || "SIGTERM") : "SIGTERM"
    stopTimeout.value = serviceData ? Number(serviceData.gracefulStopSeconds || 10) : 10
    dependencyField.text = serviceData && serviceData.dependencies ? serviceData.dependencies.map(function(dep) { return dep.serviceId + ":" + dep.condition }).join("\n") : ""
    healthType.value = serviceData && serviceData.health ? String(serviceData.health.type || "none") : "none"
    healthTarget.text = healthTargetFor(serviceData ? serviceData.health : null)
    healthInterval.value = serviceData && serviceData.health ? Number(serviceData.health.intervalSeconds || 10) : 10
    healthTimeout.value = serviceData && serviceData.health ? Number(serviceData.health.timeoutSeconds || 3) : 3
    healthRetries.value = serviceData && serviceData.health ? Number(serviceData.health.retries || 3) : 3
    healthGrace.value = serviceData && serviceData.health ? numberOr(serviceData.health.startGraceSeconds, 5) : 5
    healthExpected.value = serviceData && serviceData.health && serviceData.health.http ? Number(serviceData.health.http.expectedStatus || 200) : 200
    responseContains.text = serviceData && serviceData.health && serviceData.health.http ? String(serviceData.health.http.responseContains || "") : ""
    urlField.text = serviceData ? String(serviceData.url || "") : ""
    hostnameField.text = serviceData && serviceData.route ? String(serviceData.route.hostname || "") : ""
    routePort.value = serviceData && serviceData.route ? Number(serviceData.route.targetPort || 0) : 0
    httpsToggle.checked = false
    notesField.text = serviceData ? String(serviceData.notes || "") : ""
    dockerToggle.checked = Boolean(serviceData && serviceData.docker)
    composeFileField.text = serviceData && serviceData.docker ? String(serviceData.docker.composeFile || "") : ""
    composeServiceField.text = serviceData && serviceData.docker ? String(serviceData.docker.service || "") : ""
    opened = true
    initialDraft = JSON.stringify(buildService())
    Qt.callLater(function() { nameField.forceActiveFocus() })
  }

  function healthTargetFor(health) {
    if (!health) return ""
    if (health.http) return String(health.http.url || "")
    if (health.tcp) {
      var host = String(health.tcp.host || "127.0.0.1")
      return (host.indexOf(":") >= 0 ? "[" + host + "]" : host) + ":" + String(health.tcp.port || "")
    }
    if (health.command) return [health.command.executable].concat(health.command.arguments || []).join("\n")
    return ""
  }

  function buildHealth() {
    if (healthType.value === "none") return null
    var common = { type: healthType.value, intervalSeconds: healthInterval.value, timeoutSeconds: healthTimeout.value, retries: healthRetries.value, startGraceSeconds: healthGrace.value }
    if (healthType.value === "http" || healthType.value === "https") {
      common.http = source && source.health && source.health.http ? clone(source.health.http) : {}
      common.http.url = healthTarget.text.trim()
      common.http.expectedStatus = healthExpected.value
      common.http.responseContains = responseContains.text
    } else if (healthType.value === "tcp") {
      var target = healthTarget.text.trim()
      var split = target.lastIndexOf(":")
      var host = split >= 0 ? target.substring(0, split) : "127.0.0.1"
      if (host.charAt(0) === "[" && host.charAt(host.length-1) === "]") host = host.substring(1,host.length-1)
      common.tcp = { host: host, port: Number(split >= 0 ? target.substring(split+1) : target) }
    } else {
      if (source && source.health && source.health.command && healthTarget.text === healthTargetFor(source.health)) common.command = clone(source.health.command)
      else {
        var commandLines = healthTarget.text.split("\n")
        common.command = { executable: commandLines.shift() || "", arguments: commandLines }
      }
    }
    return common
  }

  function buildService() {
    var env = {}
    for (var index = 0; index < environmentRows.count; index++) {
      var row = environmentRows.get(index)
      if (String(row.keyText || "").trim() !== "") {
        var value = { value: row.keepFrom ? "" : row.valueText, secret: row.secretValue === true }
        if (row.keepFrom) value.keepFrom = row.keepFrom
        env[String(row.keyText).trim()] = value
      }
    }
    var dependencyLines = dependencyField.text.split("\n").filter(function(line) { return line.trim().length > 0 })
    var dependencies = dependencyLines.map(function(line) {
      var split = line.lastIndexOf(":")
      return { serviceId: split > 0 ? line.substring(0, split).trim() : line.trim(), condition: split > 0 ? line.substring(split + 1).trim() : "started" }
    })
    var originalArgs = source && source.command ? source.command.arguments || [] : []
    var args = argumentsField.text === originalArgs.join("\n") ? clone(originalArgs) : (argumentsField.text === "" ? [] : argumentsField.text.split("\n"))
    var result = {
      name: nameField.text.trim(), description: descriptionField.text.trim(),
      command: { executable: executableField.text.trim(), arguments: args },
      workingDirectory: directoryField.text.trim(), environment: env, environmentFile: envFileField.text.trim(),
      autostart: autostartToggle.checked,
      restart: { mode: restartMode.value, delaySeconds: restartDelay.value, maxAttempts: restartAttempts.value, resetAfterSeconds: source && source.restart ? numberOr(source.restart.resetAfterSeconds, 60) : 60 },
      stopSignal: stopSignal.value, gracefulStopSeconds: stopTimeout.value,
      dependencies: dependencies, url: urlField.text.trim(), notes: notesField.text,
      health: buildHealth()
    }
    if (root.editing) result.id = source.id
    if (shellToggle.checked) result.shell = { enabled: true, shell: shellPathField.text.trim() || "/bin/sh", command: shellCommandField.text }
    if (dockerToggle.checked) result.docker = { composeFile: composeFileField.text.trim(), projectName: source && source.docker ? String(source.docker.projectName || "") : "", service: composeServiceField.text.trim() }
    if (hostnameField.text.trim() !== "") result.route = { hostname: hostnameField.text.trim(), targetPort: routePort.value, https: false }
    return result
  }

  QtObject { id: shellToggle; property bool checked: false }
  QtObject { id: dockerToggle; property bool checked: false }
  QtObject { id: dependencyField; property string text: "" }

  function commandMode() {
    return dockerToggle.checked ? "compose" : (shellToggle.checked ? "shell" : "host")
  }

  function setCommandMode(mode) {
    dockerToggle.checked = mode === "compose"
    shellToggle.checked = mode === "shell"
  }

  function dependencyCondition(serviceId) {
    var lines = dependencyField.text.split("\n")
    for (var index = 0; index < lines.length; index++) {
      var prefix = serviceId + ":"
      if (lines[index].indexOf(prefix) === 0) return lines[index].substring(prefix.length) || "started"
    }
    return ""
  }

  function cycleDependency(serviceId) {
    var current = dependencyCondition(serviceId)
    var next = current === "" ? "started" : (current === "started" ? "healthy" : "")
    var lines = dependencyField.text.split("\n").filter(function(line) {
      return line.trim() !== "" && line.indexOf(serviceId + ":") !== 0
    })
    if (next !== "") lines.push(serviceId + ":" + next)
    dependencyField.text = lines.join("\n")
  }

  visible: opened
  z: 45

  Rectangle {
    anchors.fill: parent
    color: Color.popups.background
    MouseArea { anchors.fill: parent; onClicked: {} }

    Item {
      anchors.fill: parent
      anchors.margins: Style.space(12)

      Row {
        id: header
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: Style.space(32)

        Text { textFormat: Text.PlainText;
          width: parent.width - closeButton.width
          anchors.verticalCenter: parent.verticalCenter
          text: root.editing ? "Edit service" : "Add service"
          color: Color.foreground
          font.family: Style.font.family
          font.pixelSize: Style.font.title
          font.bold: true
          elide: Text.ElideRight
        }

        Button {
          id: closeButton
          width: parent.height
          height: parent.height
          iconText: "󰅖"
          tooltipText: "Cancel"
          focusable: true
          enabled: !root.saving
          onClicked: root.cancelEditor()
        }
      }

      Row {
        id: tabBar
        enabled: !root.saving
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: header.bottom
        anchors.topMargin: Style.space(6)
        height: Style.space(30)
        spacing: Style.space(3)

        Repeater {
          model: [
            { label: "Run", full: "Command" },
            { label: "Env", full: "Environment" },
            { label: "Flow", full: "Lifecycle and dependencies" },
            { label: "Check", full: "Health check" },
            { label: "Web", full: "URL and local route" },
            { label: "Notes", full: "Notes" }
          ]

          Button {
            required property int index
            required property var modelData
            width: (tabBar.width - tabBar.spacing * 5) / 6
            height: tabBar.height
            text: modelData.label
            tooltipText: modelData.full
            selected: root.page === index
            focusable: true
            fontSize: Style.font.bodySmall
            horizontalPadding: Style.space(2)
            verticalPadding: Style.space(2)
            onClicked: root.page = index
          }
        }
      }

      Layouts.StackLayout {
        id: pages
        enabled: !root.saving
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: tabBar.bottom
        anchors.bottom: saveMessage.top
        anchors.topMargin: Style.space(8)
        anchors.bottomMargin: Style.space(8)
        currentIndex: root.page

        Controls.ScrollView {
          clip: true
          contentWidth: availableWidth
          Column {
            width: parent.width
            spacing: Style.space(7)

            Row {
              width: parent.width
              height: Style.space(30)
              spacing: Style.space(4)

              Repeater {
                model: [
                  { value: "host", label: "Host", icon: "󰆍" },
                  { value: "shell", label: "Shell", icon: "󰆍" },
                  { value: "compose", label: "Compose", icon: "󰡨" }
                ]

                Button {
                  required property var modelData
                  width: (parent.width - parent.spacing * 2) / 3
                  height: parent.height
                  text: modelData.label
                  iconText: modelData.icon
                  selected: root.commandMode() === modelData.value
                  tooltipText: modelData.value === "shell" ? "Explicit shell syntax; use with care" : modelData.label + " service"
                  focusable: true
                  fontSize: Style.font.bodySmall
                  horizontalPadding: Style.space(3)
                  onClicked: root.setCommandMode(modelData.value)
                }
              }
            }

            TextField { id: nameField; width: parent.width; placeholderText: "Service name"; Accessible.name: "Service name" }
            TextField { id: descriptionField; width: parent.width; placeholderText: "Optional description"; Accessible.name: "Service description" }

            Column {
              visible: root.commandMode() === "host"
              width: parent.width
              spacing: Style.space(6)

              TextField { id: executableField; width: parent.width; placeholderText: "Executable path"; Accessible.name: "Executable path" }
              Text { textFormat: Text.PlainText; width: parent.width; text: "ARGUMENTS · ONE PER LINE"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true }
              Controls.TextArea {
                id: argumentsField
                width: parent.width
                height: Style.space(88)
                color: Color.foreground
                font.family: Style.font.family
                font.pixelSize: Style.font.bodySmall
                wrapMode: TextEdit.NoWrap
                Accessible.name: "Command arguments, one per line"
                background: BorderSurface {
                  color: Style.controlFill(argumentsField.activeFocus, argumentsField.hovered, Color.foreground, Color.accent)
                  borderSpec: Border.controlSpec(argumentsField.activeFocus ? "focus" : "normal", Color.foreground, Color.accent)
                  radius: Style.cornerRadius
                }
              }
            }

            Column {
              visible: root.commandMode() === "shell"
              width: parent.width
              spacing: Style.space(6)

              TextField { id: shellPathField; width: parent.width; placeholderText: "/bin/sh"; Accessible.name: "Shell executable" }
              Controls.TextArea {
                id: shellCommandField
                width: parent.width
                height: Style.space(104)
                color: Color.foreground
                font.family: Style.font.family
                font.pixelSize: Style.font.bodySmall
                wrapMode: TextEdit.Wrap
                placeholderText: "Shell command"
                Accessible.name: "Shell command"
                background: BorderSurface {
                  color: Style.controlFill(shellCommandField.activeFocus, shellCommandField.hovered, Color.foreground, Color.accent)
                  borderSpec: Border.controlSpec(shellCommandField.activeFocus ? "focus" : "normal", Color.foreground, Color.accent)
                  radius: Style.cornerRadius
                }
              }
              Text { textFormat: Text.PlainText; width: parent.width; text: "Shell mode enables expansion and pipes. Treat this command as trusted code."; color: Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
            }

            Column {
              visible: root.commandMode() === "compose"
              width: parent.width
              spacing: Style.space(7)

              TextField { id: composeFileField; width: parent.width; placeholderText: "Absolute compose.yaml path"; Accessible.name: "Compose file" }
              TextField { id: composeServiceField; width: parent.width; placeholderText: "Compose service name"; Accessible.name: "Compose service" }
              Text { textFormat: Text.PlainText; width: parent.width; text: "Compose manages this service. Editing and import never execute the file."; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
            }

            TextField {
              id: directoryField
              visible: root.commandMode() !== "compose"
              width: parent.width
              placeholderText: "Absolute working directory"
              Accessible.name: "Working directory"
            }
          }
        }

        Item {
          TextField {
            id: envFileField
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            placeholderText: "Optional absolute environment-file path"
            Accessible.name: "Environment file"
          }

          Row {
            id: envHeader
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: envFileField.bottom
            anchors.topMargin: Style.space(8)
            height: envHeaderText.implicitHeight

            Text { textFormat: Text.PlainText; id: envHeaderText; width: parent.width - envCount.width; text: "ENVIRONMENT VARIABLES"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true }
            Text { textFormat: Text.PlainText; id: envCount; text: environmentRows.count; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
          }

          Row {
            id: envFooter
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: addVariableButton.implicitHeight

            Button {
              id: addVariableButton
              text: "Add variable"
              iconText: "󰐕"
              focusable: true
              onClicked: environmentRows.append({ keyText: "", valueText: "", secretValue: false, keepFrom: "" })
            }
            Item { width: parent.width - addVariableButton.width - secretHint.width; height: 1 }
            Text { textFormat: Text.PlainText; id: secretHint; width: Style.space(150); anchors.verticalCenter: parent.verticalCenter; text: "Secrets masked · mode 0600"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; horizontalAlignment: Text.AlignRight; elide: Text.ElideRight }
          }

          BorderSurface {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: envHeader.bottom
            anchors.bottom: envFooter.top
            anchors.topMargin: Style.space(6)
            anchors.bottomMargin: Style.space(8)
            color: Util.alpha(Color.foreground, 0.025)
            borderSpec: Border.flat(Util.alpha(Color.foreground, 0.14), Style.normalBorderWidth)
            radius: Style.cornerRadius

            ListView {
              id: environmentList
              anchors.fill: parent
              anchors.margins: Style.space(4)
              model: environmentRows
              spacing: Style.space(4)
              clip: true
              boundsBehavior: Flickable.StopAtBounds
              interactive: contentHeight > height

              delegate: Row {
                required property int index
                required property string keyText
                required property string valueText
                required property bool secretValue
                required property string keepFrom
                width: environmentList.width
                height: Style.space(34)
                spacing: Style.space(4)

                TextField {
                  width: Style.space(88)
                  height: parent.height
                  text: keyText
                  placeholderText: "NAME"
                  verticalPadding: Style.space(3)
                  onTextEdited: environmentRows.setProperty(index, "keyText", text)
                  Accessible.name: "Environment variable name"
                }
                TextField {
                  width: parent.width - Style.space(88) - secretButton.width - removeVariable.width - parent.spacing * 3
                  height: parent.height
                  text: valueText
                  password: secretValue
                  placeholderText: keepFrom ? "Stored value kept · type to replace" : "Value"
                  verticalPadding: Style.space(3)
                  onTextEdited: { environmentRows.setProperty(index, "keepFrom", ""); environmentRows.setProperty(index, "valueText", text) }
                  Accessible.name: "Environment variable value"
                }
                Button {
                  id: secretButton
                  width: parent.height
                  height: parent.height
                  iconText: secretValue ? "󰈈" : "󰈉"
                  selected: secretValue
                  tooltipText: secretValue ? "Stop marking as secret (value kept)" : "Mark as secret (value kept)"
                  focusable: true
                  onClicked: environmentRows.setProperty(index, "secretValue", !secretValue)
                }
                Button {
                  id: removeVariable
                  width: parent.height
                  height: parent.height
                  iconText: "󰆴"
                  tooltipText: "Remove variable"
                  focusable: true
                  onClicked: environmentRows.remove(index)
                }
              }

              Text { textFormat: Text.PlainText; anchors.centerIn: parent; visible: environmentRows.count === 0; text: "No variables"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
            }
          }
        }

        Controls.ScrollView {
          clip: true
          contentWidth: availableWidth
          Column {
            id: lifecycleColumn
            width: parent.width
            spacing: Style.space(7)

            Toggle { id: autostartToggle; width: parent.width; label: "Autostart"; description: "Start once per user-manager session; daemon updates preserve stopped apps."; onClicked: checked = !checked }

            Row {
              id: restartRow
              width: parent.width
              height: Math.max(restartMode.implicitHeight, restartDelay.implicitHeight, restartAttempts.implicitHeight)
              spacing: Style.space(6)

              Dropdown {
                id: restartMode
                width: (parent.width - parent.spacing * 2) / 3
                label: "Restart"
                options: [{value:"never",label:"Never"},{value:"on-failure",label:"On failure"},{value:"always",label:"Always"}]
                onChanged: function(nextValue) { restartMode.value = nextValue }
              }
              NumberField { id: restartDelay; width: (parent.width - parent.spacing * 2) / 3; fieldWidth: width; label: "Delay (s)"; from: 0; to: 3600; onModified: function(nextValue) { restartDelay.value = nextValue } }
              NumberField { id: restartAttempts; width: (parent.width - parent.spacing * 2) / 3; fieldWidth: width; label: "Attempts · 0 ∞"; from: 0; to: 1000; onModified: function(nextValue) { restartAttempts.value = nextValue } }
            }

            Row {
              id: stopRow
              width: parent.width
              height: Math.max(stopSignal.implicitHeight, stopTimeout.implicitHeight)
              spacing: Style.space(6)

              Dropdown { id: stopSignal; width: (parent.width - parent.spacing) / 2; label: "Stop signal"; options: ["SIGTERM","SIGINT","SIGHUP","SIGQUIT"]; onChanged: function(nextValue) { stopSignal.value = nextValue } }
              NumberField { id: stopTimeout; width: (parent.width - parent.spacing) / 2; fieldWidth: width; label: "Grace (s)"; from: 1; to: 300; onModified: function(nextValue) { stopTimeout.value = nextValue } }
            }

            Text { textFormat: Text.PlainText; id: dependencyTitle; width: parent.width; text: "DEPENDENCIES · OFF → STARTED → HEALTHY"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true; elide: Text.ElideRight }

            BorderSurface {
              width: parent.width
              height: Style.space(150)
              color: Util.alpha(Color.foreground, 0.025)
              borderSpec: Border.flat(Util.alpha(Color.foreground, 0.14), Style.normalBorderWidth)
              radius: Style.cornerRadius

              ListView {
                id: dependencyList
                anchors.fill: parent
                anchors.margins: Style.space(4)
                model: root.allServices || []
                spacing: Style.space(3)
                clip: true
                boundsBehavior: Flickable.StopAtBounds
                interactive: contentHeight > height

                delegate: BorderSurface {
                  required property var modelData
                  readonly property bool ownService: Boolean(root.source && modelData.id === root.source.id)
                  readonly property string condition: root.dependencyCondition(modelData.id)
                  visible: !ownService
                  width: dependencyList.width
                  height: ownService ? 0 : Style.space(32)
                  color: condition !== "" ? Style.selectedFillFor(Color.foreground, Color.accent) : "transparent"
                  borderSpec: condition !== "" ? Border.controlSpec("selected", Color.foreground, Color.accent) : Border.none()
                  radius: Style.cornerRadius

                  Text { textFormat: Text.PlainText; anchors.left: parent.left; anchors.right: conditionText.left; anchors.verticalCenter: parent.verticalCenter; anchors.leftMargin: Style.space(7); anchors.rightMargin: Style.space(7); text: modelData.projectName + " / " + modelData.name; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall; elide: Text.ElideRight }
                  Text { textFormat: Text.PlainText; id: conditionText; anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter; anchors.rightMargin: Style.space(7); text: parent.condition === "" ? "OFF" : (parent.condition === "healthy" ? "HEALTHY" : "STARTED"); color: parent.condition === "" ? Color.muted : Color.accent; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: parent.condition !== "" }
                  MouseArea { anchors.fill: parent; hoverEnabled: true; cursorShape: Qt.PointingHandCursor; onClicked: root.cycleDependency(parent.modelData.id) }
                }

                Text { textFormat: Text.PlainText; anchors.centerIn: parent; visible: dependencyList.count <= (root.editing ? 1 : 0); text: "No other services available"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
              }
            }

            Text { textFormat: Text.PlainText; id: dependencyHint; width: parent.width; text: "Healthy needs a health check. Stop affects only the selected services, in reverse dependency order."; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
          }
        }

        Controls.ScrollView {
          clip: true
          contentWidth: availableWidth
          Column {
            width: parent.width
            spacing: Style.space(7)

            Row {
              width: parent.width
              height: Style.space(30)
              spacing: Style.space(3)

              Repeater {
                model: [{value:"none",label:"Off"},{value:"http",label:"HTTP"},{value:"https",label:"HTTPS"},{value:"tcp",label:"TCP"},{value:"command",label:"Exec"}]
                Button {
                  required property var modelData
                  width: (parent.width - parent.spacing * 4) / 5
                  height: parent.height
                  text: modelData.label
                  selected: healthType.value === modelData.value
                  focusable: true
                  fontSize: Style.font.bodySmall
                  horizontalPadding: Style.space(2)
                  onClicked: healthType.value = modelData.value
                }
              }
            }

            QtObject { id: healthType; property string value: "none" }

            Controls.TextArea {
              id: healthTarget
              visible: healthType.value !== "none"
              width: parent.width
              height: healthType.value === "command" ? Style.space(72) : Style.space(34)
              color: Color.foreground
              font.family: Style.font.family
              font.pixelSize: Style.font.bodySmall
              wrapMode: TextEdit.NoWrap
              placeholderText: healthType.value === "tcp" ? "127.0.0.1:3000" : (healthType.value === "command" ? "/usr/bin/check · arguments on following lines" : "http://127.0.0.1:3000/health")
              Accessible.name: "Health check target"
              background: BorderSurface {
                color: Style.controlFill(healthTarget.activeFocus, healthTarget.hovered, Color.foreground, Color.accent)
                borderSpec: Border.controlSpec(healthTarget.activeFocus ? "focus" : "normal", Color.foreground, Color.accent)
                radius: Style.cornerRadius
              }
            }

            Grid {
              visible: healthType.value !== "none"
              width: parent.width
              columns: 2
              columnSpacing: Style.space(7)
              rowSpacing: Style.space(6)

              Column {
                width: (parent.parent.width - parent.columnSpacing) / 2
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "INTERVAL (S)"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                NumberField { id: healthInterval; width: parent.width; fieldWidth: width; from: 1; to: 3600; onModified: function(nextValue) { healthInterval.value = nextValue } }
              }
              Column {
                width: (parent.parent.width - parent.columnSpacing) / 2
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "TIMEOUT (S)"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                NumberField { id: healthTimeout; width: parent.width; fieldWidth: width; from: 1; to: 60; onModified: function(nextValue) { healthTimeout.value = nextValue } }
              }
              Column {
                width: (parent.parent.width - parent.columnSpacing) / 2
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "RETRIES"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                NumberField { id: healthRetries; width: parent.width; fieldWidth: width; from: 1; to: 20; onModified: function(nextValue) { healthRetries.value = nextValue } }
              }
              Column {
                width: (parent.parent.width - parent.columnSpacing) / 2
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "START GRACE (S)"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                NumberField { id: healthGrace; width: parent.width; fieldWidth: width; from: 0; to: 3600; onModified: function(nextValue) { healthGrace.value = nextValue } }
              }
            }

            Row {
              visible: healthType.value === "http" || healthType.value === "https"
              width: parent.width
              height: Math.max(expectedColumn.implicitHeight, responseColumn.implicitHeight)
              spacing: Style.space(7)

              Column {
                id: expectedColumn
                width: Style.space(96)
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "STATUS"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                NumberField { id: healthExpected; width: parent.width; fieldWidth: width; from: 100; to: 599; onModified: function(nextValue) { healthExpected.value = nextValue } }
              }
              Column {
                id: responseColumn
                width: parent.width - expectedColumn.width - parent.spacing
                spacing: Style.space(3)
                Text { textFormat: Text.PlainText; width: parent.width; text: "RESPONSE CONTAINS"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption }
                TextField { id: responseContains; width: parent.width; placeholderText: "Optional text"; Accessible.name: "Health response text match" }
              }
            }

            BorderSurface {
              width: parent.width
              height: healthInfo.implicitHeight + Style.space(16)
              color: Util.alpha(Color.accent, 0.05)
              borderSpec: Border.flat(Util.alpha(Color.foreground, 0.14), Style.normalBorderWidth)
              radius: Style.cornerRadius
              Text { textFormat: Text.PlainText;
                id: healthInfo
                anchors.fill: parent
                anchors.margins: Style.space(8)
                text: healthType.value === "none" ? "No active check. Process state still reports starts, stops and crashes." : "Checks use bounded concurrency and deadlines. HTTPS verification stays enabled."
                color: Color.muted
                font.family: Style.font.family
                font.pixelSize: Style.font.caption
                wrapMode: Text.WordWrap
              }
            }
          }
        }

        Controls.ScrollView {
          clip: true
          contentWidth: availableWidth
          Column {
            width: parent.width
            spacing: Style.space(8)

            TextField { id: urlField; width: parent.width; placeholderText: "Optional URL to open"; Accessible.name: "Service URL" }
            Text { textFormat: Text.PlainText; width: parent.width; text: "LOCAL ROUTE"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true }
            TextField { id: hostnameField; width: parent.width; placeholderText: "frontend.myapp.test"; Accessible.name: "Local route hostname" }
            NumberField { id: routePort; width: parent.width; fieldWidth: parent.width; label: "Target port · 0 uses detected port"; from: 0; to: 65535; onModified: function(nextValue) { routePort.value = nextValue } }
            Toggle { id: httpsToggle; enabled: false; width: parent.width; label: "Local HTTPS · unavailable"; description: "Requires the future certificate-trust onboarding flow."; checked: false }

            BorderSurface {
              width: parent.width
              height: routeInfo.implicitHeight + Style.space(16)
              color: Util.alpha(Color.foreground, 0.035)
              borderSpec: Border.flat(Util.alpha(Color.foreground, 0.16), Style.normalBorderWidth)
              radius: Style.cornerRadius
              Text { textFormat: Text.PlainText; id: routeInfo; anchors.fill: parent; anchors.margins: Style.space(8); text: "Loopback only. OmaStack never changes DNS, hosts, firewall or certificate trust silently."; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; wrapMode: Text.WordWrap }
            }
          }
        }

        Item {
          Text { textFormat: Text.PlainText; id: notesLabel; anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top; text: "NOTES · SETUP HINTS, TEAM CONTEXT, KNOWN CAVEATS"; color: Color.muted; font.family: Style.font.family; font.pixelSize: Style.font.caption; font.bold: true; elide: Text.ElideRight }
          Controls.TextArea {
            id: notesField
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: notesLabel.bottom
            anchors.bottom: parent.bottom
            anchors.topMargin: Style.space(7)
            color: Color.foreground
            font.family: Style.font.family
            font.pixelSize: Style.font.bodySmall
            wrapMode: TextEdit.Wrap
            placeholderText: "Team notes, setup hints, known caveats…"
            Accessible.name: "Service notes"
            background: BorderSurface {
              color: Style.controlFill(notesField.activeFocus, notesField.hovered, Color.foreground, Color.accent)
              borderSpec: Border.controlSpec(notesField.activeFocus ? "focus" : "normal", Color.foreground, Color.accent)
              radius: Style.cornerRadius
            }
          }
        }
      }

      Text { textFormat: Text.PlainText;
        id: saveMessage
        anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: footer.top
        anchors.bottomMargin: Style.space(6)
        text: root.saving ? "Saving…" : root.saveError
        color: root.saveError !== "" ? Color.urgent : Color.muted
        font.family: Style.font.family; font.pixelSize: Style.font.caption
        wrapMode: Text.WordWrap
      }

      Row {
        id: footer
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: Math.max(cancelButton.implicitHeight, saveButton.implicitHeight)
        spacing: Style.space(7)

        Button { id: cancelButton; text: "Cancel"; height: parent.height; focusable: true; enabled: !root.saving; onClicked: root.cancelEditor() }
        Item { width: Math.max(0, parent.width - cancelButton.width - saveButton.width - parent.spacing * 2); height: 1 }
        Button {
          id: saveButton
          text: root.editing ? "Save" : "Add"
          iconText: "󰄬"
          tooltipText: root.editing ? "Save service changes" : "Add service"
          height: parent.height
          active: true
          focusable: true
          enabled: !root.saving
          onClicked: {
            if (nameField.text.trim() === "") { root.page = 0; nameField.forceActiveFocus(); return }
            if (!dockerToggle.checked && !shellToggle.checked && executableField.text.trim() === "") { root.page = 0; executableField.forceActiveFocus(); return }
            if (!dockerToggle.checked && directoryField.text.trim() === "") { root.page = 0; directoryField.forceActiveFocus(); return }
            root.saved(root.projectId, root.buildService(), root.editing)
          }
        }
      }
    }
  }

  ConfirmDialog {
    id: discardDialog
    anchors.fill: parent
    message: "Discard unsaved service changes?"
    confirmText: "Discard"
    onOpenedChanged: if (opened) root.forceActiveFocus()
    onCanceled: opened = false
    onConfirmed: { opened = false; root.canceled() }
  }
}
