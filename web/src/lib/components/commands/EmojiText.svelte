<script lang="ts">
	// A Discord-style text surface: plain text plus custom-emoji markup
	// (<:name:id> / <a:name:id>) drawn as real inline images, exactly like
	// Discord's own composer. The bound value keeps the wire syntax; only the
	// presentation is rich. Typing, Enter and paste are kept plain-text (the
	// element never grows block structure), so serialising is just text nodes
	// plus image tokens.
	let {
		value = '',
		onChange,
		multiline = true,
		placeholder = '',
		emojiSize = 18,
		class: cls = ''
	}: {
		value?: string;
		onChange: (next: string) => void;
		multiline?: boolean;
		placeholder?: string;
		emojiSize?: number;
		class?: string;
	} = $props();

	let el = $state<HTMLDivElement | null>(null);
	let lastEmitted: string | null = null;
	let savedRange: Range | null = null;

	const TOKEN_RE = /<(a?):([\w~-]+):(\d{15,21})>/g;

	function imgHTML(animated: string, name: string, id: string): string {
		const token = `<${animated ? 'a' : ''}:${name}:${id}>`;
		const src = `https://cdn.discordapp.com/emojis/${id}.${animated ? 'gif' : 'png'}?size=64`;
		return (
			`<img src="${src}" alt="${escapeAttr(token)}" title=":${escapeAttr(name)}:"` +
			` data-token="${escapeAttr(token)}" draggable="false"` +
			` style="display:inline-block;width:${emojiSize}px;height:${emojiSize}px;` +
			`object-fit:contain;vertical-align:-${Math.round(emojiSize * 0.22)}px;margin:0 1px;">`
		);
	}

	function escapeText(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	}
	function escapeAttr(s: string): string {
		return escapeText(s).replace(/"/g, '&quot;');
	}

	// Text with tokens, as HTML the surface renders.
	function richHTML(text: string): string {
		let out = '';
		let last = 0;
		TOKEN_RE.lastIndex = 0;
		for (const m of text.matchAll(TOKEN_RE)) {
			out += escapeText(text.slice(last, m.index));
			out += imgHTML(m[1], m[2], m[3]);
			last = (m.index ?? 0) + m[0].length;
		}
		out += escapeText(text.slice(last));
		return out;
	}

	// The DOM, back to wire text. The surface only ever holds text nodes,
	// emoji imgs and stray <br>s (some browsers leave one behind on deletes).
	function serialize(root: HTMLElement): string {
		let out = '';
		const walk = (n: Node) => {
			if (n.nodeType === Node.TEXT_NODE) {
				out += n.nodeValue ?? '';
				return;
			}
			if (n instanceof HTMLImageElement) {
				out += n.dataset.token ?? '';
				return;
			}
			if (n instanceof HTMLBRElement) {
				// A trailing <br> is presentation (it makes the final newline
				// visible), not content.
				if (n.nextSibling) out += '\n';
				return;
			}
			n.childNodes.forEach(walk);
			if (n instanceof HTMLElement && /^(DIV|P)$/.test(n.tagName) && n.nextSibling) {
				out += '\n';
			}
		};
		root.childNodes.forEach(walk);
		return out;
	}

	function emit() {
		if (!el) return;
		const text = serialize(el);
		// Fully cleared surfaces sometimes keep a stray <br>, which would
		// defeat the :empty placeholder.
		if (text === '' && el.childNodes.length > 0) el.replaceChildren();
		lastEmitted = text;
		onChange(text);
	}

	// External value changes re-render; our own input (echoed back through
	// the parent) must NOT, or the caret would reset on every keystroke.
	$effect(() => {
		const v = value ?? '';
		if (!el || v === lastEmitted) return;
		lastEmitted = v;
		el.innerHTML = richHTML(v);
	});

	// The caret survives the focus loss to a picker popover: whatever was
	// last selected inside this surface is where an insert lands.
	function rememberSelection() {
		const sel = window.getSelection();
		if (el && sel && sel.rangeCount > 0 && el.contains(sel.getRangeAt(0).startContainer)) {
			savedRange = sel.getRangeAt(0).cloneRange();
		}
	}

	function tokenImg(animated: string, name: string, id: string): HTMLImageElement {
		const img = document.createElement('img');
		img.src = `https://cdn.discordapp.com/emojis/${id}.${animated ? 'gif' : 'png'}?size=64`;
		img.alt = `<${animated ? 'a' : ''}:${name}:${id}>`;
		img.title = `:${name}:`;
		img.dataset.token = img.alt;
		img.draggable = false;
		img.style.cssText =
			`display:inline-block;width:${emojiSize}px;height:${emojiSize}px;` +
			`object-fit:contain;vertical-align:-${Math.round(emojiSize * 0.22)}px;margin:0 1px;`;
		return img;
	}

	// insertToken lands a value at the caret: emoji markup becomes an image,
	// anything else (template tokens, plain text) inserts as text. The DOM is
	// edited through the SAVED range, never the live selection: while a
	// picker popover is still open its focus trap owns the focus, and any
	// selection set under it gets yanked before an execCommand could run.
	// Exposed on the element so the composer's shared pickers can reach the
	// focused surface without prop plumbing.
	function insertToken(token: string) {
		if (!el) return;
		let range: Range;
		if (savedRange && el.contains(savedRange.startContainer)) {
			range = savedRange;
		} else {
			range = document.createRange();
			range.selectNodeContents(el);
			range.collapse(false);
		}
		const m = /^<(a?):([\w~-]+):(\d{15,21})>$/.exec(token.trim());
		const node: Node = m ? tokenImg(m[1], m[2], m[3]) : document.createTextNode(token);
		range.deleteContents();
		range.insertNode(node);
		range.setStartAfter(node);
		range.collapse(true);
		savedRange = range.cloneRange();
		emit();
		// Put the visible caret back once the popover has finished closing
		// and released the focus.
		requestAnimationFrame(() => {
			if (!el || !savedRange) return;
			el.focus();
			const sel = window.getSelection();
			if (sel && el.contains(savedRange.startContainer)) {
				sel.removeAllRanges();
				sel.addRange(savedRange);
			}
		});
	}

	type RichHost = HTMLElement & { __insertToken?: (token: string) => void };
	$effect(() => {
		if (!el) return;
		(el as RichHost).__insertToken = insertToken;
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		if (!multiline) return;
		document.execCommand('insertText', false, '\n');
		emit();
	}

	function onPaste(e: ClipboardEvent) {
		e.preventDefault();
		let text = e.clipboardData?.getData('text/plain') ?? '';
		if (!multiline) text = text.replace(/\s*\n\s*/g, ' ');
		// Pasted emoji markup renders rich right away.
		document.execCommand('insertHTML', false, richHTML(text));
		emit();
	}
</script>

<div
	bind:this={el}
	contenteditable="true"
	role="textbox"
	tabindex="0"
	aria-multiline={multiline}
	aria-label={placeholder}
	data-placeholder={placeholder}
	data-emoji-ok
	spellcheck="false"
	class="emoji-text {cls}"
	oninput={emit}
	onkeydown={onKeydown}
	onpaste={onPaste}
	onkeyup={rememberSelection}
	onmouseup={rememberSelection}
	onblur={rememberSelection}
></div>

<style>
	.emoji-text {
		background: transparent;
		border: none;
		outline: none;
		border-radius: 3px;
		white-space: pre-wrap;
		word-break: break-word;
		cursor: text;
		min-height: 1.4em;
	}
	.emoji-text:hover {
		background: rgba(255, 255, 255, 0.03);
	}
	.emoji-text:focus {
		background: rgba(255, 255, 255, 0.04);
	}
	.emoji-text:empty::before {
		content: attr(data-placeholder);
		color: #6d6f78;
		pointer-events: none;
	}
</style>
