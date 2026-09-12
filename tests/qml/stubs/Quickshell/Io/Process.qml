import QtQuick
QtObject {
  property bool running: false
  property var command: []
  property var stdout: null
  property var stderr: null
  property bool stdinEnabled: false
  property string written: ""
  signal started()
  function write(value) { written += value }
  signal exited(int exitCode, int exitStatus)
  // Intentionally does nothing: fixtures may inspect queues but cannot execute.
}
