import QtQuick

Rectangle {
    id: root

    property string glyph: ""
    property color foreground: "#d3c6aa"
    property color actionColor: foreground
    property color surfaceColor: "#343f44"
    property bool emphasized: false
    property string tooltip: ""

    signal clicked()

    width: 24
    height: 24
    radius: 0
    color: emphasized || mouse.containsMouse
        ? Qt.rgba(actionColor.r, actionColor.g, actionColor.b, emphasized ? 0.18 : 0.12)
        : Qt.rgba(foreground.r, foreground.g, foreground.b, 0.04)
    border.width: mouse.containsMouse ? 1 : 0
    border.color: Qt.rgba(actionColor.r, actionColor.g, actionColor.b, 0.42)

    Behavior on color { ColorAnimation { duration: 60 } }

    Text {
        anchors.centerIn: parent
        text: root.glyph
        color: root.actionColor
        font.family: "monospace"
        font.pixelSize: 13
    }

    MouseArea {
        id: mouse
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.clicked()
    }

    Rectangle {
        visible: root.tooltip !== "" && mouse.containsMouse
        z: 20
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.top
        anchors.bottomMargin: 6
        width: tip.implicitWidth + 16
        height: tip.implicitHeight + 10
        color: root.surfaceColor
        border.width: 1
        border.color: root.foreground
        radius: 0

        Text {
            id: tip
            anchors.centerIn: parent
            text: root.tooltip
            color: root.foreground
            font.family: "monospace"
            font.pixelSize: 10
        }
    }
}
