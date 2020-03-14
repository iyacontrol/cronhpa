package controller

import (
	"github.com/iyacontrol/cronhpa/pkg/controller/cronhpa"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, cronhpa.Add)
}
