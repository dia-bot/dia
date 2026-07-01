// Mirror of internal/features/verification/config.go. Keep json field names in lockstep.

export type VerifyMode = 'button' | 'captcha';

// One custom button's wired automation: when the button (matched by its
// custom_id_suffix) is clicked, run the saved automation. Mirrors the Go
// ButtonAction { suffix, automation_id }.
export interface VerifyButtonAction {
	suffix: string;
	automation_id: string;
}

export interface VerificationConfig {
	mode: VerifyMode;
	unverified_role: string;
	verified_role: string;
	channel: string;
	// Legacy plain-text prompt. Kept for back-compat; the backend falls back to
	// it when `content` is empty.
	welcome_text: string;
	// ── Rich gate message (mirrors the customcommands send_message surface) ──
	// content is the gate text (a Go template); embeds / components are the same
	// shapes MessageEditor writes into step.spec. The "Verify" button is NOT
	// stored in components — the backend injects it as the first button.
	content?: string;
	embeds?: unknown[];
	components?: unknown[];
	// Per-custom-button automation mapping (by custom_id_suffix).
	button_actions?: VerifyButtonAction[];
	kick_after_minutes: number;
	only_suspicious: boolean;
	min_account_age_hours?: number;
	// Flag joiners with no profile picture as suspicious (default on).
	require_avatar: boolean;
	// id of a saved automation launched when a member verifies (optional).
	run_automation?: string;
}

export function defaultVerification(): VerificationConfig {
	return {
		mode: 'button',
		unverified_role: '',
		verified_role: '',
		channel: '',
		welcome_text:
			'Welcome to {{ .Guild.Name }}! Click the button below to verify you are human and unlock the server.',
		content: '',
		embeds: [],
		components: [],
		button_actions: [],
		kick_after_minutes: 0,
		only_suspicious: false,
		min_account_age_hours: 24,
		require_avatar: true,
		run_automation: ''
	};
}
