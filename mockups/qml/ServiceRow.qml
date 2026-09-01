import QtQuick

Item {
    id: root

    property string serviceName: "Service"
    property string status: "running"
    property string metricsText: ""
    property string portsText: ""
    property string domainText: ""
    property string detailText: ""
    property string errorText: ""
    property bool showControls: false
    property bool chartExpanded: false
    property bool panelOpen: true
    property var cpuSamples: []
    property var memorySamples: []
    property var themeColors

    signal chartToggled()

    readonly property color statusColor: {
        if (status === "running") return themeColors.green
        if (status === "unhealthy") return themeColors.orange
        if (status === "crashed") return themeColors.red
        if (status === "starting" || status === "stopping") return themeColors.yellow
        return themeColors.muted
    }
    readonly property bool hasDetail: detailText !== "" || domainText !== ""
    readonly property bool hasError: errorText !== ""
    readonly property int rowHeight: 30
    readonly property int detailHeight: hasDetail ? 17 : 0
    readonly property int errorHeight: hasError ? 28 : 0
    readonly property int chartHeight: chartExpanded ? 108 : 0

    implicitHeight: rowHeight + detailHeight + errorHeight + chartHeight

    Rectangle {
        anchors.fill: parent
        color: mouse.containsMouse || root.showControls
            ? Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.045)
            : "transparent"
        radius: 0
    }

    Rectangle {
        x: 11
        y: 12
        width: 6
        height: 6
        radius: 3
        color: root.statusColor
    }

    Text {
        x: 27
        y: 7
        text: root.serviceName
        color: root.themeColors.fg
        font.family: "monospace"
        font.pixelSize: 11
        font.weight: Font.Medium
    }

    Row {
        visible: !root.showControls
        anchors.right: parent.right
        anchors.rightMargin: 8
        y: 7
        spacing: 8

        Text {
            visible: root.metricsText !== ""
            text: root.metricsText
            color: root.themeColors.muted
            font.family: "monospace"
            font.pixelSize: 9
        }
        Text {
            visible: root.portsText !== ""
            text: root.portsText
            color: root.themeColors.cyan
            font.family: "monospace"
            font.pixelSize: 9
        }
    }

    Row {
        visible: root.showControls
        anchors.right: parent.right
        anchors.rightMargin: 6
        y: 3
        spacing: 3

        IconButton {
            visible: root.status === "running" || root.status === "unhealthy"
            glyph: "󰜉"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.accent
            surfaceColor: root.themeColors.surface
            tooltip: "Restart"
        }
        IconButton {
            visible: root.status === "running" || root.status === "unhealthy"
            glyph: "󰓛"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.red
            surfaceColor: root.themeColors.surface
            tooltip: "Stop"
        }
        IconButton {
            visible: root.status === "stopped" || root.status === "crashed"
            glyph: "󰐊"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.green
            surfaceColor: root.themeColors.surface
            tooltip: "Start"
        }
        IconButton {
            glyph: "󰈙"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.muted
            surfaceColor: root.themeColors.surface
            tooltip: "Logs"
        }
    }

    Row {
        visible: root.hasDetail
        x: 27
        y: root.rowHeight
        spacing: 7

        Text {
            visible: root.detailText !== ""
            text: root.detailText
            color: root.status === "unhealthy" ? root.themeColors.orange : root.themeColors.muted
            font.family: "monospace"
            font.pixelSize: 9
        }
        Text {
            visible: root.domainText !== ""
            text: "󰖟  " + root.domainText
            color: root.themeColors.accent
            font.family: "monospace"
            font.pixelSize: 9
        }
    }

    Row {
        visible: root.hasError
        x: 27
        y: root.rowHeight + root.detailHeight + 2
        spacing: 5

        Text {
            text: "󰀪"
            color: root.themeColors.red
            font.family: "monospace"
            font.pixelSize: 10
        }
        Text {
            width: root.width - 58
            text: root.errorText
            color: root.themeColors.red
            opacity: 0.88
            elide: Text.ElideRight
            font.family: "monospace"
            font.pixelSize: 9
        }
    }

    Rectangle {
        visible: root.chartExpanded
        x: 27
        y: root.rowHeight + root.detailHeight + root.errorHeight + 3
        width: root.width - 35
        height: 98
        color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.025)
        border.width: 1
        border.color: Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.12)
        radius: 0

        Row {
            x: 9
            y: 6
            spacing: 14
            Text {
                text: "━ CPU %"
                color: root.themeColors.green
                font.family: "monospace"
                font.pixelSize: 9
                font.weight: Font.DemiBold
            }
            Text {
                text: "━ Memory MB"
                color: root.themeColors.accent
                font.family: "monospace"
                font.pixelSize: 9
                font.weight: Font.DemiBold
            }
        }

        Sparkline {
            x: 9
            y: 24
            width: parent.width - 18
            height: 48
            cpuSamples: root.cpuSamples
            memorySamples: root.memorySamples
            cpuColor: root.themeColors.green
            memoryColor: root.themeColors.accent
            gridColor: root.themeColors.muted
            tooltipBackground: root.themeColors.surface
            tooltipText: root.themeColors.fg
            updatesEnabled: root.panelOpen
        }

        Text {
            x: 9
            anchors.bottom: parent.bottom
            anchors.bottomMargin: 7
            text: "●  3.8%    ●  86 MB"
            color: root.themeColors.muted
            font.family: "monospace"
            font.pixelSize: 9
        }
        Text {
            anchors.right: parent.right
            anchors.rightMargin: 9
            anchors.bottom: parent.bottom
            anchors.bottomMargin: 7
            text: "60 samples · 5m"
            color: root.themeColors.muted
            opacity: 0.72
            font.family: "monospace"
            font.pixelSize: 8
        }
    }

    MouseArea {
        id: mouse
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.chartToggled()
    }
}
