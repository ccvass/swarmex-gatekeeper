package gatekeeper

import (
	"github.com/docker/docker/api/types/filters"
)

func serviceFilter(serviceID string) filters.Args {
	f := filters.NewArgs()
	f.Add("service", serviceID)
	return f
}
