# traffic-deflector

traffic-deflector serves as a health check target. It detects which node sent
the request and fails the health check is drained.

This is most useful if you're running your (nginx) ingress as a DaemonSet. By
using traffic-deflector, nodes that are about to go down (drained) will stop
receiving new traffic before they actually go down.
