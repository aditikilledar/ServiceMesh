package main

func (ctr *ControlPlane) getRouteMapping(serviceName string) string {
	return ctr.routes[serviceName]
}
