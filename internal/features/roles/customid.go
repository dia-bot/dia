package roles

import (
	"strconv"
	"strings"
)

// custom_id routing for reaction-role components. All ids share the "rr:" prefix
// (registered once with reg.Component):
//
//	rr:btn:<menuID>:<roleID>   button click (one role per button)
//	rr:sel:<menuID>            string-select submit (roles in ComponentValues)
const (
	componentPrefix = "rr:"
	buttonPrefix    = "rr:btn:"
	selectPrefix    = "rr:sel:"
)

// buttonID builds a button custom_id for a menu option role.
func buttonID(menuID int64, roleID string) string {
	return buttonPrefix + strconv.FormatInt(menuID, 10) + ":" + roleID
}

// selectID builds a string-select custom_id for a menu.
func selectID(menuID int64) string {
	return selectPrefix + strconv.FormatInt(menuID, 10)
}

// parseButtonID extracts (menuID, roleID) from a button custom_id.
func parseButtonID(customID string) (menuID int64, roleID string, ok bool) {
	rest, found := strings.CutPrefix(customID, buttonPrefix)
	if !found {
		return 0, "", false
	}
	menu, role, found := strings.Cut(rest, ":")
	if !found || menu == "" || role == "" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(menu, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, role, true
}

// parseSelectID extracts the menuID from a string-select custom_id.
func parseSelectID(customID string) (menuID int64, ok bool) {
	rest, found := strings.CutPrefix(customID, selectPrefix)
	if !found || rest == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
