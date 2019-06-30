// InfoMark - a platform for managing courses with
//            distributing exercise sheets and testing exercise submissions
// Copyright (C) 2019  Patrick Wieschollek
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"log"
	"strconv"

	"github.com/cgtuebingen/infomark-cli/bridge"
)

// H is a neat alias
type H map[string]interface{}

func MustInt64Parameter(argStr string, name string) int64 {
	argInt, err := strconv.Atoi(argStr)
	if err != nil {
		log.Fatalf("cannot convert %s '%s' to int64\n", name, argStr)
		return int64(0)
	}
	return int64(argInt)
}

func MustIntParameter(argStr string, name string) int {
	argInt, err := strconv.Atoi(argStr)
	if err != nil {
		log.Fatalf("cannot convert %s '%s' to int\n", name, argStr)
		return int(0)
	}
	return int(argInt)
}

var conn *bridge.Connection
var remote *bridge.Bridge
