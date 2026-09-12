import QtQuick

// Replacement bars intentionally cannot retrieve other plugins' service
// objects. In that case this widget owns an ordinary client of our daemon.
Item {
  id: root
  visible: false
  property var hostShell: null
  readonly property var sharedService: hostShell && typeof hostShell.serviceFor === "function"
    ? hostShell.serviceFor("david.omastack") : null
  readonly property var service: sharedService || localClient.item

  Loader {
    id: localClient
    active: !root.sharedService
    sourceComponent: Component { Service { manageIpc: false } }
  }
}
