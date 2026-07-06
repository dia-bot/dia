// Starter automation templates for the create flow. Each entry is a ready-made
// trigger + step program the user can drop in and finish (fill the one channel
// or role a template deliberately leaves blank). Every user-facing string is a
// Go text/template rendered against the run's scope, matching the templating
// contract used everywhere else in the editor.
//
// definition is a FACTORY (not a value) so newStep() mints fresh step ids on
// every creation — templates never share step-id references between automations.

import { newStep, type Definition, type Step } from '$lib/commands/types';
import type { TriggerConfig } from './types';

export interface AutomationTemplate {
	key: string;
	name: string;
	description: string;
	trigger_type: string;
	trigger_config: TriggerConfig;
	// Fresh Definition on every call so step ids are never reused.
	definition: () => Definition;
}

// step mints a leaf step with a fresh id (via newStep) and a curated spec,
// replacing newStep's placeholder default so no legacy brace sugar leaks in.
function step(kind: string, spec: Record<string, unknown>): Step {
	const s = newStep(kind);
	s.spec = spec;
	return s;
}

export const AUTOMATION_TEMPLATES: AutomationTemplate[] = [
	{
		key: 'thank-booster',
		name: 'Thank a booster',
		description: 'When a member gets the Server Booster role, post a public thank-you.',
		trigger_type: 'role_added',
		// Leave role blank so validation walks the user to pick the booster role.
		trigger_config: { role: '' },
		definition: () => ({
			steps: [
				step('send_message', {
					channel: { src: '' },
					content:
						'Thanks for boosting {{ .Guild.Name }}, {{ .User.Mention }}! Your support keeps the community going.'
				})
			]
		})
	},
	{
		key: 'welcome-dm',
		name: 'Welcome DM',
		description: 'DM every new member a friendly welcome and a few pointers.',
		trigger_type: 'member_join',
		trigger_config: { ignore_bots: true },
		definition: () => ({
			steps: [
				step('send_dm', {
					user: { src: '{{ .User.ID }}' },
					content:
						'Welcome to {{ .Guild.Name }}, {{ .User.GlobalName }}! Glad to have you. Read the rules, grab a role, and say hi in the chat.'
				})
			]
		})
	},
	{
		key: 'automod-log',
		name: 'Automod log',
		description: 'Post an embed to a moderation channel whenever an automod rule fires.',
		trigger_type: 'automod_action',
		trigger_config: {},
		definition: () => ({
			steps: [
				step('send_message', {
					channel: { src: '' },
					embeds: [
						{
							title: 'Automod: {{ .Event.rule_name }}',
							description: '{{ .Event.reason }}',
							color: '#ff6363',
							fields: [
								{ name: 'Member', value: '{{ .User.Mention }}', inline: true },
								{ name: 'Trigger', value: '{{ .Event.trigger_type }}', inline: true },
								{
									name: 'Points',
									value: '{{ .Event.points }} (total {{ .Event.total_points }})',
									inline: true
								},
								{ name: 'Channel', value: '<#{{ .Event.channel_id }}>', inline: true }
							],
							footer_text: 'Rule {{ .Event.rule_id }}',
							timestamp: true
						}
					]
				})
			]
		})
	},
	{
		key: 'deleted-message-log',
		name: 'Deleted-message log',
		description: 'Log to a channel whenever a message is deleted.',
		trigger_type: 'message_delete',
		trigger_config: {},
		definition: () => ({
			steps: [
				step('send_message', {
					channel: { src: '' },
					embeds: [
						{
							title: 'Message deleted',
							description: 'A message was deleted in <#{{ .Event.message.channel_id }}>.',
							color: '#ff6363',
							fields: [
								{ name: 'Message ID', value: '{{ .Event.message.id }}', inline: true },
								{ name: 'Channel', value: '<#{{ .Event.message.channel_id }}>', inline: true }
							],
							timestamp: true
						}
					]
				})
			]
		})
	},
	{
		key: 'auto-thread-support',
		name: 'Auto-thread support posts',
		description: 'Open a thread on every new post in your support channel so replies stay tidy.',
		trigger_type: 'message_create',
		// Add a channel filter on the trigger to scope this to your support channel.
		trigger_config: { ignore_bots: true },
		definition: () => ({
			steps: [
				step('thread_create', {
					// The channel the message landed in — threads hang off it.
					channel: { src: '{{ .Channel.ID }}' },
					message: { src: '{{ .Event.message.id }}' },
					name: 'Support: {{ .User.GlobalName }}',
					auto_archive_minutes: 1440,
					into: 'thread'
				})
			]
		})
	},
	{
		key: 'reaction-role-nudge',
		name: 'Role menu follow-up',
		description: 'DM members a short note after they pick roles from a reaction-role menu.',
		trigger_type: 'reaction_role_pick',
		trigger_config: {},
		definition: () => ({
			steps: [
				step('send_dm', {
					user: { src: '{{ .User.ID }}' },
					content:
						'Thanks for picking your roles from "{{ .Event.menu_title }}", {{ .User.GlobalName }}. You can change them any time from the same menu.'
				})
			]
		})
	}
];
