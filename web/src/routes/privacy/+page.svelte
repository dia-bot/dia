<script lang="ts">
	import LegalDoc from '$lib/components/marketing/LegalDoc.svelte';

	let { data }: { data: { user?: { username: string } | null } } = $props();

	const sections = [
		{ id: 'who', num: '01', title: 'Who we are' },
		{ id: 'scope', num: '02', title: 'Scope of this policy' },
		{ id: 'data', num: '03', title: 'Data we collect' },
		{ id: 'use', num: '04', title: 'How we use your data' },
		{ id: 'legal-basis', num: '05', title: 'Legal bases' },
		{ id: 'storage', num: '06', title: 'Storage and security' },
		{ id: 'sharing', num: '07', title: 'Sharing with third parties' },
		{ id: 'retention', num: '08', title: 'Data retention' },
		{ id: 'rights', num: '09', title: 'Your rights' },
		{ id: 'transfers', num: '10', title: 'International transfers' },
		{ id: 'children', num: '11', title: 'Children' },
		{ id: 'self-hosting', num: '12', title: 'Self-hosted instances' },
		{ id: 'changes', num: '13', title: 'Changes to this policy' },
		{ id: 'contact', num: '14', title: 'Contact' }
	];
</script>

<LegalDoc
	title="Privacy Policy"
	kind="privacy"
	description="How Dia collects, uses, stores, and protects data from Discord — including OAuth identity, server configuration, and the member data the bot processes to provide its features."
	user={data.user ?? null}
	{sections}
>
	<section id="who">
		<h2>01. Who we are</h2>
		<p>
			The hosted Dia service is operated by Mindroot Ltd, a company registered in England and Wales
			with company number 16543299, registered office 71–75 Shelton Street, London, England, WC2H 9JQ.
			For personal data we process about you when you use the hosted Service, Mindroot Ltd is the data
			controller.
		</p>
	</section>

	<section id="scope">
		<h2>02. Scope of this policy</h2>
		<p>
			This policy describes how the hosted Dia service handles data through the bot, the web
			dashboard, and the marketing website. It does not cover Discord itself, which is governed by
			Discord's own privacy policy, nor does it cover instances of Dia that other people self-host (see
			Section 12).
		</p>
	</section>

	<section id="data">
		<h2>03. Data we collect</h2>
		<h3>Account and authentication</h3>
		<ul>
			<li>Your Discord user ID, username, avatar, and the list of servers you can manage — received from Discord via OAuth when you log in to the dashboard.</li>
			<li>A session cookie and a CSRF token so the dashboard can keep you signed in securely.</li>
		</ul>
		<h3>Server configuration</h3>
		<ul>
			<li>The settings you create for a server: welcome cards, leveling rules, role menus, automod rules, custom commands, log-channel choices, and related options.</li>
			<li>Discord channel and role identifiers referenced by that configuration.</li>
		</ul>
		<h3>Member data processed to provide features</h3>
		<ul>
			<li>Discord user IDs and message metadata needed to award experience points and compute levels and leaderboards.</li>
			<li>Moderation cases — the target user ID, the acting moderator's ID, the action, an optional reason, and a timestamp.</li>
			<li>Reaction-role assignments and the IDs of posted role menus.</li>
			<li>Dia does not store the content of ordinary messages. AutoMod evaluates message content in transit to apply the rules you configure; it is not retained beyond what is needed to act on a rule.</li>
		</ul>
		<h3>Diagnostics</h3>
		<ul>
			<li>Limited logs and error reports, including IP address and request timing, retained for security and abuse prevention.</li>
		</ul>
	</section>

	<section id="use">
		<h2>04. How we use your data</h2>
		<ul>
			<li>To operate the bot and dashboard and to provide the features you enable.</li>
			<li>To award XP, render welcome and rank cards, assign roles, and keep your configuration in sync with your server in real time.</li>
			<li>To maintain the moderation case log at your direction.</li>
			<li>To secure the Service and prevent abuse and fraud.</li>
			<li>To respond to support requests and comply with legal obligations.</li>
		</ul>
		<p>We do not use your data for advertising, and we do not sell it.</p>
	</section>

	<section id="legal-basis">
		<h2>05. Legal bases for processing</h2>
		<ul>
			<li><strong>Contract.</strong> Processing necessary to provide the Service you have chosen to use.</li>
			<li><strong>Legitimate interest.</strong> Keeping the platform secure, preventing abuse, and improving the product.</li>
			<li><strong>Legal obligation.</strong> Where processing is required to comply with applicable law.</li>
		</ul>
	</section>

	<section id="storage">
		<h2>06. Storage and security</h2>
		<p>
			Durable configuration and feature data are stored in PostgreSQL. Sessions, caches, and live
			server-state snapshots are held in Redis. Discord OAuth tokens are stored only as needed to
			operate the dashboard and are not shared with anyone. All traffic between you and the Service is
			protected in transit by TLS, and access to production systems is restricted.
		</p>
	</section>

	<section id="sharing">
		<h2>07. Sharing with third parties</h2>
		<p>
			We share data only with the providers we depend on to run the Service — Discord (the platform
			the bot operates on) and our hosting and infrastructure providers — and only to the extent needed
			to operate it. We will disclose data when required by law, when you consent, or when necessary to
			investigate or stop abuse. We do not sell personal data or share it with advertisers.
		</p>
	</section>

	<section id="retention">
		<h2>08. Data retention</h2>
		<ul>
			<li>Server configuration and feature data are retained while the bot is in your server. When the bot is removed, the associated data for that server is deleted or anonymised within a reasonable period, except where retention is required by law.</li>
			<li>Discord OAuth tokens and sessions are deleted when they expire or when you log out.</li>
			<li>Diagnostic logs are retained only as long as needed for security and abuse prevention.</li>
		</ul>
		<p>You can also request deletion of a server's data at any time — see Section 09.</p>
	</section>

	<section id="rights">
		<h2>09. Your rights</h2>
		<p>
			Subject to applicable law, you have the right to access, rectify, delete, restrict, port, and
			object to the processing of your personal data, and to lodge a complaint with a supervisory
			authority. In the UK, that authority is the Information Commissioner's Office (ICO). To exercise
			your rights, email <a href="mailto:hello@dia.xyz">hello@dia.xyz</a>.
		</p>
	</section>

	<section id="transfers">
		<h2>10. International transfers</h2>
		<p>
			The Service is operated from, and uses infrastructure providers that may process data outside,
			the United Kingdom and the European Economic Area. Where personal data is transferred outside the
			UK or EEA, we rely on appropriate safeguards such as the UK International Data Transfer Agreement,
			the EU Standard Contractual Clauses, or an adequacy decision where one applies.
		</p>
	</section>

	<section id="children">
		<h2>11. Children</h2>
		<p>
			Discord requires users to be at least 13, or older where local law sets a higher age. The
			Service is not directed at children below that age, and we do not knowingly collect their data.
			If you believe a child has provided personal data to the hosted Service, contact us and we will
			remove it.
		</p>
	</section>

	<section id="self-hosting">
		<h2>12. Self-hosted instances</h2>
		<p>
			Dia is open source and can be self-hosted. If you use an instance hosted by someone other than
			Mindroot Ltd, that operator — not us — is the data controller for the data it processes, and this
			policy does not apply to their deployment. Questions about a self-hosted instance should go to
			whoever operates it.
		</p>
	</section>

	<section id="changes">
		<h2>13. Changes to this policy</h2>
		<p>
			We may update this policy from time to time. The "Last updated" date above reflects the most
			recent change. If we make material changes, we will provide reasonable notice through the
			dashboard or our repository before they take effect.
		</p>
	</section>

	<section id="contact">
		<h2>14. Contact</h2>
		<p>
			For privacy questions or data requests, email <a href="mailto:hello@dia.xyz">hello@dia.xyz</a> or
			write to Mindroot Ltd, 71–75 Shelton Street, London, England, WC2H 9JQ.
		</p>
	</section>
</LegalDoc>
