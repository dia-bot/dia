package tickets

import "strings"

// componentPrefix namespaces every ticketing component + modal custom_id. Ids
// carry all the state they need (panel id, category id, ticket id, guild id for
// DM rating) so the buttons stay valid across worker restarts.
const componentPrefix = "tkt:"

// Panel actions (persistent on the panel message).
func openButtonID(panelID, categoryID string) string {
	return componentPrefix + "open:" + panelID + ":" + categoryID
}
func selectMenuID(panelID string) string { return componentPrefix + "sel:" + panelID }
func formModalID(panelID, categoryID string) string {
	return componentPrefix + "form:" + panelID + ":" + categoryID
}

// In-ticket controls (persistent on the opening / closed message).
func closeButtonID(ticketID string) string      { return componentPrefix + "close:" + ticketID }
func closeModalID(ticketID string) string       { return componentPrefix + "closeform:" + ticketID }
func claimButtonID(ticketID string) string      { return componentPrefix + "claim:" + ticketID }
func unclaimButtonID(ticketID string) string    { return componentPrefix + "unclaim:" + ticketID }
func reopenButtonID(ticketID string) string     { return componentPrefix + "reopen:" + ticketID }
func deleteButtonID(ticketID string) string     { return componentPrefix + "delete:" + ticketID }
func transcriptButtonID(ticketID string) string { return componentPrefix + "transcript:" + ticketID }

// Close-request controls (opener confirms or declines a staff close request).
func closeReqAcceptID(ticketID string) string { return componentPrefix + "crok:" + ticketID }
func closeReqDenyID(ticketID string) string   { return componentPrefix + "crno:" + ticketID }

// actionButtonID routes a composed (non-link) ticket button back here:
// tkt:act:<ticketID>:<suffix>. The click runs the saved automation the
// category's ButtonActions maps the suffix to.
func actionButtonID(ticketID, suffix string) string {
	return componentPrefix + "act:" + ticketID + ":" + suffix
}

// panelActionID routes a composed panel button that is wired to an automation
// (rather than opening a ticket): tkt:pact:<panelID>:<suffix>.
func panelActionID(panelID, suffix string) string {
	return componentPrefix + "pact:" + panelID + ":" + suffix
}

// Rating select, DMed to the opener on close. The guild id is embedded because a
// DM interaction carries no guild (mirrors welcome's welcome:dm:* scheme).
func rateSelectID(guildID, ticketID string) string {
	return componentPrefix + "rate:" + guildID + ":" + ticketID
}

// parseID strips the "tkt:" prefix and splits the remainder into its action and
// up to two arguments (the grammar never needs more).
func parseID(customID string) (action string, args []string) {
	rest := strings.TrimPrefix(customID, componentPrefix)
	parts := strings.SplitN(rest, ":", 4)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
