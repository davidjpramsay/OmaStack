import QtQuick

Item {
    id: root

    property var cpuSamples: []
    property var memorySamples: []
    property color cpuColor: "#a7c080"
    property color memoryColor: "#7fbbb3"
    property color gridColor: "#475258"
    property color tooltipBackground: "#21272c"
    property color tooltipText: "#d3c6aa"
    property bool updatesEnabled: true
    property int maxSamples: 60

    property int hoverIndex: -1

    function bounded(values) {
        if (!values || values.length <= maxSamples)
            return values || []
        return values.slice(values.length - maxSamples)
    }

    function maxValue(a, b) {
        var result = 1
        var aa = bounded(a)
        var bb = bounded(b)
        for (var i = 0; i < aa.length; ++i)
            result = Math.max(result, Number(aa[i]) || 0)
        for (var j = 0; j < bb.length; ++j)
            result = Math.max(result, Number(bb[j]) || 0)
        return Math.ceil(result / 10) * 10
    }

    function drawSeries(ctx, samples, color, fillAlpha, maxY, width, height) {
        var values = bounded(samples)
        if (values.length < 2)
            return

        var step = width / (values.length - 1)
        ctx.beginPath()
        for (var i = 0; i < values.length; ++i) {
            var x = i * step
            var y = height - Math.max(0, Number(values[i]) || 0) / maxY * height
            if (i === 0)
                ctx.moveTo(x, y)
            else
                ctx.lineTo(x, y)
        }
        ctx.lineTo(width, height)
        ctx.lineTo(0, height)
        ctx.closePath()
        ctx.fillStyle = Qt.rgba(color.r, color.g, color.b, fillAlpha)
        ctx.fill()

        ctx.beginPath()
        for (var j = 0; j < values.length; ++j) {
            var px = j * step
            var py = height - Math.max(0, Number(values[j]) || 0) / maxY * height
            if (j === 0)
                ctx.moveTo(px, py)
            else
                ctx.lineTo(px, py)
        }
        ctx.strokeStyle = color
        ctx.lineWidth = 1.5
        ctx.lineJoin = "round"
        ctx.lineCap = "round"
        ctx.stroke()
    }

    Canvas {
        id: canvas
        anchors.fill: parent
        antialiasing: true

        onPaint: {
            var ctx = getContext("2d")
            ctx.reset()
            var maxY = root.maxValue(root.cpuSamples, root.memorySamples)

            ctx.strokeStyle = Qt.rgba(root.gridColor.r, root.gridColor.g, root.gridColor.b, 0.32)
            ctx.lineWidth = 0.5
            ctx.setLineDash([2, 3])
            for (var i = 0; i < 3; ++i) {
                var y = Math.round(i * height / 2) + 0.5
                ctx.beginPath()
                ctx.moveTo(0, y)
                ctx.lineTo(width, y)
                ctx.stroke()
            }
            ctx.setLineDash([])

            root.drawSeries(ctx, root.memorySamples, root.memoryColor, 0.08, maxY, width, height)
            root.drawSeries(ctx, root.cpuSamples, root.cpuColor, 0.08, maxY, width, height)

            if (root.hoverIndex >= 0) {
                var count = Math.max(root.bounded(root.cpuSamples).length, root.bounded(root.memorySamples).length)
                if (count > 1) {
                    var hx = root.hoverIndex * width / (count - 1)
                    ctx.strokeStyle = Qt.rgba(root.tooltipText.r, root.tooltipText.g, root.tooltipText.b, 0.42)
                    ctx.lineWidth = 1
                    ctx.beginPath()
                    ctx.moveTo(hx + 0.5, 0)
                    ctx.lineTo(hx + 0.5, height)
                    ctx.stroke()
                }
            }
        }
    }

    onCpuSamplesChanged: if (updatesEnabled) canvas.requestPaint()
    onMemorySamplesChanged: if (updatesEnabled) canvas.requestPaint()
    onUpdatesEnabledChanged: if (updatesEnabled) canvas.requestPaint()

    MouseArea {
        anchors.fill: parent
        hoverEnabled: true
        onPositionChanged: function(mouse) {
            var count = Math.max(root.bounded(root.cpuSamples).length, root.bounded(root.memorySamples).length)
            root.hoverIndex = count > 1 ? Math.max(0, Math.min(count - 1, Math.round(mouse.x / width * (count - 1)))) : -1
            canvas.requestPaint()
        }
        onExited: {
            root.hoverIndex = -1
            canvas.requestPaint()
        }
    }

    Rectangle {
        visible: root.hoverIndex >= 0
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 4
        width: hoverText.implicitWidth + 10
        height: hoverText.implicitHeight + 6
        color: root.tooltipBackground
        border.width: 1
        border.color: Qt.rgba(root.tooltipText.r, root.tooltipText.g, root.tooltipText.b, 0.28)
        radius: 0

        Text {
            id: hoverText
            anchors.centerIn: parent
            color: root.tooltipText
            font.family: "monospace"
            font.pixelSize: 9
            text: {
                if (root.hoverIndex < 0)
                    return ""
                var cpu = root.bounded(root.cpuSamples)
                var mem = root.bounded(root.memorySamples)
                var c = root.hoverIndex < cpu.length ? cpu[root.hoverIndex] : 0
                var m = root.hoverIndex < mem.length ? mem[root.hoverIndex] : 0
                return Number(c).toFixed(1) + "% · " + Math.round(m) + " MB"
            }
        }
    }
}
