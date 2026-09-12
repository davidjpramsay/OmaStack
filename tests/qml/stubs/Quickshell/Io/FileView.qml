import QtQuick
QtObject {
  property string path: ""
  property bool watchChanges: false
  property bool printErrors: false
  property bool preload: false
  signal loaded()
  signal fileChanged()
  signal loadFailed()
  function reload() {}
  function text() { return "" }
}
