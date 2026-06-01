package interactions

import "github.com/dia-bot/dia/pkg/discordgo"

// Slash builds a chat-input application command.
func Slash(name, description string, opts ...*discordgo.ApplicationCommandOption) *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Type:        discordgo.ChatApplicationCommand,
		Name:        name,
		Description: description,
		Options:     opts,
	}
}

// AdminOnly restricts a command to members with Manage Server by default.
// Server admins can still override per-command visibility in Discord's UI.
func AdminOnly(cmd *discordgo.ApplicationCommand) *discordgo.ApplicationCommand {
	p := int64(discordgo.PermissionManageServer)
	cmd.DefaultMemberPermissions = &p
	return cmd
}

// RequirePerms restricts a command to members holding the given permission bits.
func RequirePerms(cmd *discordgo.ApplicationCommand, perms int64) *discordgo.ApplicationCommand {
	cmd.DefaultMemberPermissions = &perms
	return cmd
}

// SubCommand builds a sub-command option.
func SubCommand(name, description string, opts ...*discordgo.ApplicationCommandOption) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Name:        name,
		Description: description,
		Options:     opts,
	}
}

// SubCommandGroup builds a sub-command group option.
func SubCommandGroup(name, description string, subs ...*discordgo.ApplicationCommandOption) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
		Name:        name,
		Description: description,
		Options:     subs,
	}
}

// StringOpt builds a string option.
func StringOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionString, name, description, required)
}

// IntOpt builds an integer option.
func IntOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionInteger, name, description, required)
}

// BoolOpt builds a boolean option.
func BoolOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionBoolean, name, description, required)
}

// UserOpt builds a user option.
func UserOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionUser, name, description, required)
}

// RoleOpt builds a role option.
func RoleOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionRole, name, description, required)
}

// ChannelOpt builds a channel option.
func ChannelOpt(name, description string, required bool) *discordgo.ApplicationCommandOption {
	return opt(discordgo.ApplicationCommandOptionChannel, name, description, required)
}

// WithChoices attaches a fixed choice set to a string/int option.
func WithChoices(o *discordgo.ApplicationCommandOption, choices ...*discordgo.ApplicationCommandOptionChoice) *discordgo.ApplicationCommandOption {
	o.Choices = choices
	return o
}

// WithAutocomplete enables autocomplete on an option.
func WithAutocomplete(o *discordgo.ApplicationCommandOption) *discordgo.ApplicationCommandOption {
	o.Autocomplete = true
	return o
}

// Choice builds an option choice.
func Choice(name string, value any) *discordgo.ApplicationCommandOptionChoice {
	return &discordgo.ApplicationCommandOptionChoice{Name: name, Value: value}
}

func opt(t discordgo.ApplicationCommandOptionType, name, description string, required bool) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{Type: t, Name: name, Description: description, Required: required}
}
