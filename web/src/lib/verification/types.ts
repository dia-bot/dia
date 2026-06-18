// Mirror of internal/features/verification/config.go. Keep json field names in lockstep.

export type VerifyMode = 'button' | 'captcha';

export interface VerificationConfig {
	mode: VerifyMode;
	unverified_role: string;
	verified_role: string;
	channel: string;
	welcome_text: string;
	kick_after_minutes: number;
	only_suspicious: boolean;
	min_account_age_hours?: number;
}

export function defaultVerification(): VerificationConfig {
	return {
		mode: 'button',
		unverified_role: '',
		verified_role: '',
		channel: '',
		welcome_text:
			'Welcome to {{ .Guild.Name }}! Click the button below to verify you are human and unlock the server.',
		kick_after_minutes: 0,
		only_suspicious: false,
		min_account_age_hours: 24
	};
}
