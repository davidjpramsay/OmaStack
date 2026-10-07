import QtQuick
import QtTest
import qs.Commons
import "../../qml" as App

Rectangle {
  id: stage
  width: 800
  height: panel.height * 2 + 40
  color: Color.popups.background

  App.CompactPanel {
    id: panel
    x: 20; y: 20
    width: 380
    height: implicitHeight
    scale: 2
    transformOrigin: Item.TopLeft
    expandedProjectId: "project"
    selectedServiceId: "web"
    panelVisible: true
  }

  TestCase {
    name: "MarketplacePreview"
    when: windowShown
    function test_capture() {
      Style.resolvedFontFamily = "JetBrainsMono Nerd Font"
      Color.loadColors('mode="dark"\nbackground="#2d353b"\nforeground="#d3c6aa"\naccent="#7fbbb3"\nred="#e67e80"\nmuted="#475258"\nselection="#3d484d"')
      var history = []
      for (var i=0; i<40; i++) history.push({cpu:Math.max(0,12+15*Math.sin(i/4)),memoryMb:180+Math.sin(i/6)*10})
      panel.service = {
        connected:true, runningCount:2, stoppedCount:0, unhealthyCount:0, crashedCount:0,
        projects:[{id:"project",name:"Example project",icon:"󰆍",services:[
          {id:"web",name:"Web frontend",url:"http://127.0.0.1:3000"},
          {id:"api",name:"Application API",url:"http://127.0.0.1:8000"}]}],
        runtime:{web:{status:"running",cpu:12.5,memoryMb:189,ports:[3000],history:history},api:{status:"running",cpu:1.2,memoryMb:64,ports:[8000]}},
        snapshot:{settings:{},routes:[],diagnostics:[]},
        lastError:"", actionStatus:"", composeImport:[], logs:[], logsLoading:false,
        setPanelVisible:function(value) {}, refresh:function() {}
      }
      panel.expandedProjectId = "project"
      panel.selectedServiceId = "web"
      wait(250)
      var capture = grabImage(stage)
      compare(capture.width,800)
      compare(capture.height,stage.height)
      capture.save("preview.png")
    }
  }
}
