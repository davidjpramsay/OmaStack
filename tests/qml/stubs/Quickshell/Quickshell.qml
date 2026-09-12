pragma Singleton
import QtQuick
QtObject {
  function env(name) { return "" }
  function execDetached(command) { throw new Error("Process execution is forbidden in QML tests") }
}
