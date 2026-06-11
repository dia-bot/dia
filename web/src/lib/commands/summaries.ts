// One-line human summaries for steps — shown on canvas cards and in the
// step drawer header. Pure formatting over the step's spec; no validation.
import type { Step } from './types';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnySpec = any;

const exprSrc = (v: unknown): string =>
	v && typeof v === 'object' && 'src' in (v as Record<string, unknown>)
		? ((v as { src?: string }).src ?? '')
		: '';

const trunc = (v: unknown, n = 44): string => {
	const x = String(v ?? '');
	return x.length > n ? x.slice(0, n - 1) + '…' : x;
};

export function stepSummary(s: Step): string {
	const spec = (s.spec ?? {}) as AnySpec;
	switch (s.kind) {
		case 'reply':
		case 'edit_reply':
			return spec.content ? trunc(spec.content) : '';
		case 'defer_reply':
			return spec.ephemeral ? 'ephemeral' : '';
		case 'send_message':
			return `→ ${exprSrc(spec.channel)} · ${trunc(spec.content ?? '')}`;
		case 'send_dm':
			return `→ ${exprSrc(spec.user)}`;
		case 'embed_send':
			return spec.embed?.title ? trunc(spec.embed.title) : `→ ${exprSrc(spec.channel)} · embed`;
		case 'modal_open':
			return `prompt: ${spec.title || '(untitled)'}`;
		case 'message_edit':
			return `${spec.target === 'reply' ? 'the reply' : exprSrc(spec.message) || 'message'} · ${trunc(spec.content ?? '', 28)}`;
		case 'message_fetch':
			return `${exprSrc(spec.message) || 'message'} → ${spec.into ?? '?'}`;
		case 'message_delete':
			return exprSrc(spec.message);
		case 'message_purge':
			return `up to ${spec.limit || 50} in ${exprSrc(spec.channel) || 'channel'}`;
		case 'message_crosspost':
			return exprSrc(spec.message) || 'message';
		case 'react_add':
		case 'react_remove':
			return `${spec.emoji ?? '👍'} on ${exprSrc(spec.message) || 'message'}`;
		case 'react_clear':
			return `${spec.emoji || 'all'} on ${exprSrc(spec.message) || 'message'}`;
		case 'pin_add':
		case 'pin_remove':
			return exprSrc(spec.message);
		case 'role_add':
		case 'role_remove':
			return `${exprSrc(spec.user)} · ${exprSrc(spec.role) || '(pick a role)'}`;
		case 'member_nickname':
			return `${exprSrc(spec.user)} → ${trunc(spec.nickname ?? '', 24)}`;
		case 'member_kick':
		case 'member_unban':
			return exprSrc(spec.user);
		case 'member_fetch':
			return `${exprSrc(spec.user)} → ${spec.into ?? '?'}`;
		case 'voice_set': {
			const bits: string[] = [];
			if (spec.mute !== undefined) bits.push(spec.mute ? 'mute' : 'unmute');
			if (spec.deafen !== undefined) bits.push(spec.deafen ? 'deafen' : 'undeafen');
			return `${bits.join(' + ') || '?'} ${exprSrc(spec.user)}`;
		}
		case 'thread_member':
			return `${spec.action ?? 'add'} ${exprSrc(spec.user)} ${spec.action === 'remove' ? 'from' : 'to'} ${exprSrc(spec.thread) || 'thread'}`;
		case 'invite_create':
			return `${exprSrc(spec.channel) || 'channel'} → ${spec.into ?? 'invite'}`;
		case 'member_ban':
			return `${exprSrc(spec.user)}${spec.reason ? ` · ${trunc(spec.reason, 24)}` : ''}`;
		case 'member_timeout':
			return `${exprSrc(spec.user)} · ${spec.duration ?? '?'}`;
		case 'channel_create':
			return `#${spec.name ?? 'new-channel'} (${spec.type ?? 'text'})`;
		case 'channel_edit':
		case 'channel_delete':
			return exprSrc(spec.channel);
		case 'thread_create':
			return trunc(spec.name ?? '', 32);
		case 'thread_archive':
			return exprSrc(spec.thread);
		case 'voice_move':
			return `${exprSrc(spec.user)} → ${exprSrc(spec.channel) || 'disconnect'}`;
		case 'image_render':
			return `template #${spec.template_id ?? 0} → ${spec.into ?? '?'}`;
		case 'image_attach':
			return `${spec.from_var ?? '?'} as ${spec.filename ?? 'file'}`;
		case 'image_load':
			return trunc(exprSrc(spec.source), 36);
		case 'set_var':
			return `${spec.name ?? '?'} = ${trunc(exprSrc(spec.value), 28)}`;
		case 'incr_var':
			return `${spec.name ?? '?'} += ${spec.by ?? 0}`;
		case 'pick_random':
			return `${trunc(exprSrc(spec.from), 26) || '?'} → ${spec.into ?? '?'}`;
		case 'json_parse':
			return `${trunc(exprSrc(spec.value), 26) || '?'} → ${spec.into ?? '?'}`;
		case 'kv_get':
		case 'kv_set':
		case 'kv_delete':
			return `${spec.scope ?? 'guild'}:${spec.key ?? '?'}`;
		case 'http_request':
			return `${(spec.method ?? 'GET').toUpperCase()} ${trunc(spec.url ?? '', 36)}`;
		case 'if':
			return trunc(exprSrc(spec.cond), 40);
		case 'switch':
			return trunc(exprSrc(spec.on), 40);
		case 'loop':
			return `${spec.as ?? 'item'} ∈ ${trunc(exprSrc(spec.over), 32) || '?'}`;
		case 'parallel': {
			const n = (spec.branches ?? []).length;
			return `${n} branch${n === 1 ? '' : 'es'} · join ${spec.join ?? 'all'}`;
		}
		case 'wait':
			return `pause ${spec.duration ?? '?'}`;
		case 'wait_for':
			return `await ${spec.trigger ?? '?'}${spec.custom_id_suffix ? `:${spec.custom_id_suffix}` : ''} · ${spec.timeout ?? ''}`;
		case 'exit':
			return spec.reason || '';
		case 'fail':
			return spec.message || '';
		case 'run_command':
			return `/${spec.command || '?'}`;
		case 'audit_note':
			return spec.action || '';
	}
	return '';
}
