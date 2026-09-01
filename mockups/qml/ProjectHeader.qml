import QtQuick

Item {
    id: root

    property string projectName: "Project"
    property string status: "Running"
    property int serviceCount: 1
    property string glyph: "󰆍"
    property bool expanded: false
    property bool showControls: false
    property color statusColor: themeColors.green
    property var themeColors

    signal toggled()

    implicitHeight: 46

    Rectangle {
        anchors.fill: parent
        color: mouse.containsMouse
            ? Qt.rgba(root.themeColors.fg.r, root.themeColors.fg.g, root.themeColors.fg.b, 0.05)
            : "transparent"
        radius: 0
    }

    Rectangle {
        x: 4
        y: 8
        width: 30
        height: 30
        color: Qt.rgba(root.statusColor.r, root.statusColor.g, root.statusColor.b, 0.10)
        border.width: 1
        border.color: Qt.rgba(root.statusColor.r, root.statusColor.g, root.statusColor.b, 0.18)
        radius: 0

        Text {
            anchors.centerIn: parent
            text: root.glyph
            color: root.statusColor
            font.family: "monospace"
            font.pixelSize: 15
        }
    }

    Text {
        x: 45
        y: 7
        text: root.projectName
        color: root.themeColors.fg
        font.family: "monospace"
        font.pixelSize: 12
        font.weight: Font.DemiBold
    }

    Row {
        x: 45
        y: 25
        spacing: 5
        Rectangle {
            anchors.verticalCenter: parent.verticalCenter
            width: 5
            height: 5
            radius: 3
            color: root.statusColor
        }
        Text {
            text: root.status
            color: root.statusColor
            font.family: "monospace"
            font.pixelSize: 9
            font.weight: Font.DemiBold
        }
        Text {
            text: "· " + root.serviceCount + (root.serviceCount === 1 ? " service" : " services")
            color: root.themeColors.muted
            font.family: "monospace"
            font.pixelSize: 9
        }
    }

    Row {
        visible: root.showControls
        anchors.right: chevron.left
        anchors.rightMargin: 7
        y: 10
        spacing: 3

        IconButton {
            glyph: "󰜉"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.accent
            surfaceColor: root.themeColors.surface
            tooltip: "Restart all"
        }
        IconButton {
            glyph: "󰓛"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.red
            surfaceColor: root.themeColors.surface
            tooltip: "Stop all"
        }
        IconButton {
            glyph: "󰈙"
            foreground: root.themeColors.fg
            actionColor: root.themeColors.muted
            surfaceColor: root.themeColors.surface
            tooltip: "Project logs"
        }
    }

    Text {
        id: chevron
        anchors.right: parent.right
        anchors.rightMargin: 9
        y: 17
        text: root.expanded ? "󰅀" : "󰅂"
        color: root.themeColors.muted
        font.family: "monospace"
        font.pixelSize: 10
    }

    MouseArea {
        id: mouse
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.toggled()
    }
}
