import QtQuick
import QtQuick.Window

Window {
    id: root

    function argument(name, fallback) {
        var prefix = name + "="
        for (var i = 0; i < Qt.application.arguments.length; ++i) {
            var arg = String(Qt.application.arguments[i])
            var prefixAt = arg.indexOf(prefix)
            if (prefixAt >= 0)
                return arg.substring(prefixAt + prefix.length)
        }
        return fallback
    }

    function colorValue(hex) {
        var value = String(hex).replace("#", "")
        return Qt.rgba(parseInt(value.substring(0, 2), 16) / 255,
                       parseInt(value.substring(2, 4), 16) / 255,
                       parseInt(value.substring(4, 6), 16) / 255, 1)
    }

    function resolvedPalette(name) {
        var source = palettes[name] || palettes.everforest
        var result = { label: source.label }
        for (var key in source) {
            if (key !== "label")
                result[key] = colorValue(source[key])
        }
        return result
    }

    property string themeName: argument("theme", "everforest")
    property string outputPath: argument("output", "docs/assets/mockups/omastack-" + themeName + ".png")
    property var palettes: ({
        "everforest": {
            label: "Everforest",
            bg: "#181d20",
            panel: "#2d353b",
            surface: "#343f44",
            surfaceAlt: "#21272c",
            fg: "#d3c6aa",
            muted: "#7a8587",
            accent: "#7fbbb3",
            red: "#e67e80",
            yellow: "#dbbc7f",
            orange: "#e09d7f",
            green: "#a7c080",
            cyan: "#83c092"
        },
        "tokyo-night": {
            label: "Tokyo Night",
            bg: "#0e0e14",
            panel: "#1a1b26",
            surface: "#24283b",
            surfaceAlt: "#13141c",
            fg: "#c0caf5",
            muted: "#727b9d",
            accent: "#7aa2f7",
            red: "#f7768e",
            yellow: "#e0af68",
            orange: "#eb927b",
            green: "#9ece6a",
            cyan: "#449dab"
        },
        "catppuccin-latte": {
            label: "Catppuccin Latte",
            bg: "#d7d8dc",
            panel: "#eff1f5",
            surface: "#dce0e8",
            surfaceAlt: "#e3e4e8",
            fg: "#4c4f69",
            muted: "#8b8fa2",
            accent: "#1e66f5",
            red: "#d20f39",
            yellow: "#df8e1d",
            orange: "#d84e2b",
            green: "#40a02b",
            cyan: "#179299"
        }
    })
    property var themeColors: resolvedPalette(themeName)
    property bool panelOpen: true
    property bool qbrExpanded: true
    property bool meshExpanded: true
    property bool chartExpanded: true

    width: 480
    height: 760
    visible: true
    color: themeColors.bg
    title: "OmaStack QML mock-up — " + themeColors.label

    readonly property var cpuFixture: [3,4,4,6,7,8,7,6,8,10,12,10,9,11,13,15,14,12,11,12,10,9,8,9,11,10,9,8,7,8,9,11,13,14,12,10,9,8,9,11,12,13,11,9,8,7,8,9,7,6,5,6,7,8,7,6,5,4,4,3.8]
    readonly property var memoryFixture: [68,69,69,70,70,71,71,72,73,74,75,76,76,77,78,79,80,81,81,82,83,84,84,85,86,87,88,88,89,90,90,91,92,93,93,94,94,95,95,96,97,97,96,95,94,93,92,91,90,89,88,88,87,87,86,86,86,86,86,86]

    Rectangle {
        id: stageLabel
        x: 30
        y: 14
        width: labelText.implicitWidth + 14
        height: 18
        color: root.themeColors.surfaceAlt
        border.width: 1
        border.color: Qt.rgba(root.themeColors.accent.r, root.themeColors.accent.g,
                              root.themeColors.accent.b, 0.55)
        radius: 0

        Text {
            id: labelText
            anchors.centerIn: parent
            text: "OMARCHY · " + root.themeColors.label.toUpperCase() + " · FIXTURE"
            color: root.themeColors.accent
            font.family: "monospace"
            font.pixelSize: 9
            font.weight: Font.DemiBold
            font.letterSpacing: 0.6
        }
    }

    Rectangle {
        id: panelShadow
        x: 32
        y: 38
        width: 380
        height: 698
        color: Qt.rgba(0, 0, 0, root.themeName === "catppuccin-latte" ? 0.10 : 0.24)
        radius: 0
    }

    Rectangle {
        id: panel
        x: 30
        y: 36
        width: 380
        height: 698
        color: root.themeColors.panel
        border.width: 2
        border.color: root.themeColors.accent
        radius: 0
        clip: true

        Item {
            id: header
            x: 18
            y: 14
            width: parent.width - 36
            height: 46

            Text {
                text: "󰆍"
                color: root.themeColors.accent
                font.family: "monospace"
                font.pixelSize: 22
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
            }

            Column {
                x: 34
                y: 3
                spacing: 1
                Text {
                    text: "OmaStack"
                    color: root.themeColors.fg
                    font.family: "monospace"
                    font.pixelSize: 14
                    font.weight: Font.Bold
                }
                Text {
                    text: "Local service manager"
                    color: root.themeColors.muted
                    font.family: "monospace"
                    font.pixelSize: 10
                }
            }

            Row {
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                spacing: 6
                IconButton {
                    glyph: "󰐕"
                    foreground: root.themeColors.fg
                    actionColor: root.themeColors.accent
                    surfaceColor: root.themeColors.surface
                    tooltip: "Add project"
                }
                IconButton {
                    glyph: "󰒓"
                    foreground: root.themeColors.fg
                    actionColor: root.themeColors.muted
                    surfaceColor: root.themeColors.surface
                    tooltip: "Settings"
                }
            }
        }

        Item {
            id: summary
            x: 18
            y: 60
            width: parent.width - 36
            height: 30

            Row {
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                spacing: 13
                Row {
                    spacing: 5
                    Rectangle { width: 6; height: 6; radius: 3; color: root.themeColors.green; anchors.verticalCenter: parent.verticalCenter }
                    Text { text: "2 running"; color: root.themeColors.muted; font.family: "monospace"; font.pixelSize: 10 }
                }
                Row {
                    spacing: 5
                    Rectangle { width: 6; height: 6; radius: 3; color: root.themeColors.orange; anchors.verticalCenter: parent.verticalCenter }
                    Text { text: "4 attention"; color: root.themeColors.muted; font.family: "monospace"; font.pixelSize: 10 }
                }
                Row {
                    spacing: 5
                    Rectangle { width: 6; height: 6; radius: 3; color: root.themeColors.muted; anchors.verticalCenter: parent.verticalCenter }
                    Text { text: "2 stopped"; color: root.themeColors.muted; font.family: "monospace"; font.pixelSize: 10 }
                }
            }
            Text {
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                text: "8 total"
                color: root.themeColors.muted
                opacity: 0.74
                font.family: "monospace"
                font.pixelSize: 10
            }
        }

        Rectangle {
            id: search
            x: 18
            y: 92
            width: parent.width - 36
            height: 34
            color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.04)
            border.width: 1
            border.color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.26)
            radius: 0

            Text {
                x: 10
                anchors.verticalCenter: parent.verticalCenter
                text: "󰍉"
                color: root.themeColors.muted
                font.family: "monospace"
                font.pixelSize: 13
            }
            TextInput {
                x: 32
                width: parent.width - 42
                anchors.verticalCenter: parent.verticalCenter
                text: ""
                color: root.themeColors.fg
                font.family: "monospace"
                font.pixelSize: 11

                Text {
                    visible: parent.text.length === 0
                    text: "Search projects and services…"
                    color: root.themeColors.muted
                    font: parent.font
                }
            }
        }

        Rectangle {
            x: 0
            y: 137
            width: parent.width
            height: 1
            color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.10)
        }

        Flickable {
            id: projectScroll
            x: 14
            y: 140
            width: parent.width - 28
            height: 514
            contentWidth: width
            contentHeight: projects.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds

            Column {
                id: projects
                width: parent.width
                spacing: 1

                ProjectHeader {
                    width: parent.width
                    projectName: "QBR"
                    status: "Unhealthy"
                    serviceCount: 3
                    glyph: "󰆍"
                    expanded: root.qbrExpanded
                    showControls: true
                    statusColor: root.themeColors.orange
                    themeColors: root.themeColors
                    onToggled: root.qbrExpanded = !root.qbrExpanded
                }

                Column {
                    visible: root.qbrExpanded
                    width: parent.width
                    x: 8

                    ServiceRow {
                        width: parent.width - 8
                        serviceName: "Frontend"
                        status: "running"
                        metricsText: "2% · 148MB"
                        portsText: ":3000"
                        domainText: "frontend.qbr.test"
                        showControls: false
                        themeColors: root.themeColors
                    }
                    ServiceRow {
                        width: parent.width - 8
                        serviceName: "API"
                        status: "unhealthy"
                        metricsText: "14% · 86MB"
                        portsText: ":3001"
                        detailText: "󰋼 HTTP 503 · /health"
                        showControls: false
                        themeColors: root.themeColors
                    }
                    ServiceRow {
                        width: parent.width - 8
                        serviceName: "Postgres"
                        status: "running"
                        metricsText: "4% · 86MB"
                        portsText: ":5432"
                        showControls: false
                        chartExpanded: root.chartExpanded
                        panelOpen: root.panelOpen
                        cpuSamples: root.cpuFixture
                        memorySamples: root.memoryFixture
                        themeColors: root.themeColors
                        onChartToggled: root.chartExpanded = !root.chartExpanded
                    }
                }

                Rectangle { width: parent.width; height: 1; color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.08) }

                ProjectHeader {
                    width: parent.width
                    projectName: "Mesh"
                    status: "Crashed"
                    serviceCount: 1
                    glyph: "󰆍"
                    expanded: root.meshExpanded
                    statusColor: root.themeColors.red
                    themeColors: root.themeColors
                    onToggled: root.meshExpanded = !root.meshExpanded
                }
                Column {
                    visible: root.meshExpanded
                    width: parent.width
                    x: 8
                    ServiceRow {
                        width: parent.width - 8
                        serviceName: "Desktop"
                        status: "crashed"
                        errorText: "Process exited with error (exit 1)"
                        showControls: true
                        themeColors: root.themeColors
                    }
                }

                Rectangle { width: parent.width; height: 1; color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.08) }

                ProjectHeader {
                    width: parent.width
                    projectName: "Pulse"
                    status: "Starting"
                    serviceCount: 2
                    glyph: "󰆍"
                    expanded: false
                    statusColor: root.themeColors.yellow
                    themeColors: root.themeColors
                }

                Rectangle { width: parent.width; height: 1; color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.08) }

                ProjectHeader {
                    width: parent.width
                    projectName: "Atlas"
                    status: "Stopped"
                    serviceCount: 2
                    glyph: "󰆍"
                    expanded: false
                    statusColor: root.themeColors.muted
                    themeColors: root.themeColors
                }
            }
        }

        Rectangle {
            id: footer
            x: 0
            anchors.bottom: parent.bottom
            width: parent.width
            height: 42
            color: root.themeColors.surfaceAlt
            border.width: 0

            Rectangle {
                anchors.top: parent.top
                width: parent.width
                height: 1
                color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.12)
            }
            Text {
                x: 18
                anchors.verticalCenter: parent.verticalCenter
                text: "4 projects"
                color: root.themeColors.muted
                font.family: "monospace"
                font.pixelSize: 10
            }
            Row {
                anchors.right: parent.right
                anchors.rightMargin: 18
                anchors.verticalCenter: parent.verticalCenter
                spacing: 8
                Text {
                    text: "󰐕  Add project"
                    color: root.themeColors.accent
                    font.family: "monospace"
                    font.pixelSize: 10
                    font.weight: Font.DemiBold
                }
                Text {
                    text: "·"
                    color: root.themeColors.muted
                    font.family: "monospace"
                    font.pixelSize: 10
                }
                Text {
                    text: "󰒓  Settings"
                    color: root.themeColors.muted
                    font.family: "monospace"
                    font.pixelSize: 10
                }
            }
        }
    }

    Timer {
        id: captureTimer
        interval: 500
        repeat: false
        onTriggered: {
            root.contentItem.grabToImage(function(result) {
                if (!result.saveToFile(root.outputPath))
                    console.error("Failed to save mock-up to " + root.outputPath)
                Qt.quit()
            })
        }
    }

    Component.onCompleted: {
        if (outputPath !== "")
            captureTimer.start()
    }
}
