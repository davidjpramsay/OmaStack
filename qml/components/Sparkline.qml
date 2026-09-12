import QtQuick
import qs.Commons

Item {
  id: root
  property var samples: []
  property string valueKey: "cpu"
  property color lineColor: Color.accent
  property int maxSamples: 60
  property bool updatesEnabled: true

  function bounded() {
    var values = root.samples || []
    return values.length > root.maxSamples ? values.slice(values.length - root.maxSamples) : values
  }

  function repaint() { if (updatesEnabled) canvas.requestPaint() }
  onSamplesChanged: repaint()
  onValueKeyChanged: repaint()
  onMaxSamplesChanged: repaint()
  onWidthChanged: repaint()
  onHeightChanged: repaint()
  onUpdatesEnabledChanged: if (updatesEnabled) repaint()

  Canvas {
    id: canvas
    anchors.fill: parent
    antialiasing: true

    function clamp(value, minimum, maximum) {
      return Math.max(minimum, Math.min(maximum, value))
    }

    function traceSmooth(context, points, minimumY, maximumY) {
      if (points.length === 0) return
      context.moveTo(points[0].x, points[0].y)
      for (var index = 0; index < points.length - 1; index++) {
        var p0 = points[Math.max(0, index - 1)]
        var p1 = points[index]
        var p2 = points[index + 1]
        var p3 = points[Math.min(points.length - 1, index + 2)]
        context.bezierCurveTo(
          p1.x + (p2.x - p0.x) / 6,
          clamp(p1.y + (p2.y - p0.y) / 6, minimumY, maximumY),
          p2.x - (p3.x - p1.x) / 6,
          clamp(p2.y - (p3.y - p1.y) / 6, minimumY, maximumY),
          p2.x,
          p2.y
        )
      }
    }

    onPaint: {
      var context = getContext("2d")
      context.reset()
      var values = root.bounded()
      if (values.length < 2) return
      var plotTop = 1
      var plotBottom = Math.max(plotTop, height - 1)
      var plotHeight = plotBottom - plotTop
      var maximum = 1
      for (var i = 0; i < values.length; i++) maximum = Math.max(maximum, Number(values[i][root.valueKey] || 0))
      maximum = Math.ceil(maximum / 10) * 10
      context.strokeStyle = Util.alpha(Color.popups.text, 0.24)
      context.lineWidth = 0.75
      context.setLineDash([2, 3])
      for (var grid = 0; grid < 3; grid++) {
        var gy = Math.round(plotTop + grid * plotHeight / 2) + 0.5
        context.beginPath(); context.moveTo(0, gy); context.lineTo(width, gy); context.stroke()
      }
      context.setLineDash([])
      var step = width / (values.length - 1)
      var points = []
      for (var point = 0; point < values.length; point++) {
        points.push({
          x: point * step,
          y: plotBottom - Math.max(0, Number(values[point][root.valueKey] || 0)) / maximum * plotHeight
        })
      }

      context.beginPath()
      traceSmooth(context, points, plotTop, plotBottom)
      context.lineTo(width, plotBottom); context.lineTo(0, plotBottom); context.closePath()
      var fill = context.createLinearGradient(0, 0, 0, height)
      fill.addColorStop(0, Util.alpha(root.lineColor, 0.28))
      fill.addColorStop(1, Util.alpha(root.lineColor, 0.05))
      context.fillStyle = fill
      context.fill()

      context.beginPath()
      traceSmooth(context, points, plotTop, plotBottom)
      context.strokeStyle = root.lineColor
      context.lineWidth = 1.75
      context.lineCap = "round"
      context.lineJoin = "round"
      context.stroke()
    }
  }
}
