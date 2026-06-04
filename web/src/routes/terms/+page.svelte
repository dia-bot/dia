<script lang="ts">
	import LegalDoc from '$lib/components/marketing/LegalDoc.svelte';

	let { data }: { data: { user?: { username: string } | null } } = $props();

	const sections = [
		{ id: 'acceptance', num: '01', title: 'Acceptance of terms' },
		{ id: 'who-we-are', num: '02', title: 'Who we are' },
		{ id: 'definitions', num: '03', title: 'Definitions' },
		{ id: 'eligibility', num: '04', title: 'Eligibility and Discord' },
		{ id: 'account', num: '05', title: 'Your account and the dashboard' },
		{ id: 'use', num: '06', title: 'Acceptable use' },
		{ id: 'your-server', num: '07', title: 'Running the bot in your server' },
		{ id: 'service', num: '08', title: 'Availability and changes' },
		{ id: 'data', num: '09', title: 'Your content and data' },
		{ id: 'open-source', num: '10', title: 'Open source and self-hosting' },
		{ id: 'ip', num: '11', title: 'Intellectual property' },
		{ id: 'third-party', num: '12', title: 'Third-party services' },
		{ id: 'warranties', num: '13', title: 'Disclaimers' },
		{ id: 'liability', num: '14', title: 'Limitation of liability' },
		{ id: 'indemnity', num: '15', title: 'Indemnification' },
		{ id: 'termination', num: '16', title: 'Termination' },
		{ id: 'law', num: '17', title: 'Governing law' },
		{ id: 'changes', num: '18', title: 'Changes to these terms' },
		{ id: 'contact', num: '19', title: 'Contact' }
	];
</script>

<LegalDoc
	title="Terms of Service"
	kind="terms"
	description="The terms that govern your use of Dia: acceptable use, your responsibilities when running the bot, account rules, open-source licensing, and service limitations."
	user={data.user ?? null}
	{sections}
>
	<section id="acceptance">
		<h2>01. Acceptance of terms</h2>
		<p>
			These terms of service (the "Terms") govern your access to and use of Dia, an open-source
			Discord bot and accompanying web dashboard operated by Mindroot Ltd ("Dia", "we", "us"). By
			adding the bot to a Discord server, logging in to the dashboard, or otherwise using the
			service, you agree to be bound by these Terms. If you do not agree, do not use the service.
		</p>
		<p>
			If you use the service on behalf of a community or organisation, you confirm that you have
			authority to bind it, and "you" refers to both you and that organisation.
		</p>
	</section>

	<section id="who-we-are">
		<h2>02. Who we are</h2>
		<p>
			Dia is operated by Mindroot Ltd, a company registered in England and Wales with company number
			16543299. Our registered office is at 71–75 Shelton Street, London, England, WC2H 9JQ. You can
			reach us at <a href="mailto:hello@dia.xyz">hello@dia.xyz</a> for any matter arising under these
			Terms.
		</p>
	</section>

	<section id="definitions">
		<h2>03. Definitions</h2>
		<ul>
			<li>
				<strong>Service</strong> means the hosted Dia bot, the web dashboard, the APIs, and any related
				software or content we make available.
			</li>
			<li>
				<strong>Server</strong> means a Discord guild to which the bot has been added.
			</li>
			<li>
				<strong>Configuration</strong> means the settings you create through the dashboard or slash
				commands — welcome cards, leveling rules, role menus, automod rules, custom commands, and similar.
			</li>
			<li>
				<strong>Member data</strong> means the Discord identifiers and activity the bot processes to
				provide features, such as user, channel, and role IDs, experience points, and moderation cases.
			</li>
		</ul>
	</section>

	<section id="eligibility">
		<h2>04. Eligibility and Discord</h2>
		<p>
			Your use of the Service is in addition to, and must comply with, the
			<a href="https://discord.com/terms" target="_blank" rel="noopener noreferrer">Discord Terms of
				Service</a> and
			<a href="https://discord.com/guidelines" target="_blank" rel="noopener noreferrer">Community
				Guidelines</a>. You must meet Discord's minimum age requirement for your country (at least 13)
			to use Discord and therefore the Service.
		</p>
		<p>
			Dia is not affiliated with, endorsed by, or sponsored by Discord Inc. "Discord" is a trademark
			of Discord Inc.
		</p>
	</section>

	<section id="account">
		<h2>05. Your account and the dashboard</h2>
		<p>
			The dashboard authenticates you through Discord using OAuth. You are responsible for your
			Discord account and for all activity that happens under it within the Service. You may only
			configure a server through the dashboard if you hold the appropriate Discord permissions
			(typically Manage Server) for that server.
		</p>
		<p>
			You must keep your access secure and notify us promptly if you believe your account or a
			server's configuration has been accessed without authorisation.
		</p>
	</section>

	<section id="use">
		<h2>06. Acceptable use</h2>
		<p>You agree not to use the Service to:</p>
		<ul>
			<li>Break the law, or facilitate unlawful, harassing, hateful, defamatory, or fraudulent activity.</li>
			<li>Violate the Discord Terms of Service, Community Guidelines, or Developer Policy.</li>
			<li>Distribute malware, phishing links, or content that sexualises minors.</li>
			<li>Configure custom commands, messages, or automod responses that harass, dox, or deceive members.</li>
			<li>
				Probe, scan, or test the vulnerability of the Service, abuse our APIs, evade rate limits, or
				attempt to disrupt the infrastructure that other communities rely on.
			</li>
			<li>Resell, sublicense, or pass off the hosted Service as your own.</li>
		</ul>
		<p>
			We may suspend, throttle, or remove the bot from any server, and disable any configuration, that
			we reasonably believe breaches these rules or puts the Service or other communities at risk.
		</p>
	</section>

	<section id="your-server">
		<h2>07. Running the bot in your server</h2>
		<p>
			You decide how Dia behaves in your server. You are responsible for the configuration you create
			and for any moderation actions — bans, kicks, timeouts, warnings, message deletions, and automod
			rules — taken through the bot at your direction. You are responsible for informing your members,
			where required, about logging and automated moderation.
		</p>
		<p>
			Features such as automod operate on the rules you set. We provide the tooling; the decisions and
			their consequences within your community are yours.
		</p>
	</section>

	<section id="service">
		<h2>08. Availability and changes</h2>
		<p>
			The hosted Service is currently provided free of charge. We work to keep it available and
			reliable, but it is provided without uptime guarantees. We may add, modify, or discontinue
			features, and we may introduce paid plans for future capabilities; where we do, the terms of any
			paid plan will be made clear before you opt in.
		</p>
	</section>

	<section id="data">
		<h2>09. Your content and data</h2>
		<p>
			You retain ownership of your Configuration. You grant us a limited, non-exclusive licence to
			host, process, and display it solely to operate the Service for you and to comply with law. For
			details on what we process and why, see our <a href="/privacy">Privacy Policy</a>.
		</p>
	</section>

	<section id="open-source">
		<h2>10. Open source and self-hosting</h2>
		<p>
			Dia's source code is released under the MIT licence and is available on
			<a href="https://github.com/dia-bot/dia" target="_blank" rel="noopener noreferrer">GitHub</a>.
			You are free to self-host the software under that licence.
		</p>
		<p>
			If you self-host Dia, you operate your own instance: you are the controller of any data it
			processes, you are responsible for its security and for compliance with applicable law and with
			Discord's policies, and these Terms govern only the hosted Service we operate — not your
			deployment.
		</p>
	</section>

	<section id="ip">
		<h2>11. Intellectual property</h2>
		<p>
			Subject to the MIT licence covering the source code, Mindroot Ltd retains all rights in the Dia
			name, logo, and brand. Nothing in these Terms transfers your intellectual property to us, except
			for the limited operational licence in Section 09.
		</p>
	</section>

	<section id="third-party">
		<h2>12. Third-party services</h2>
		<p>
			The Service depends on third parties, including Discord and our hosting and infrastructure
			providers. Your use of Discord is governed by Discord's own terms, and we are not responsible for
			the availability or actions of any third-party service.
		</p>
	</section>

	<section id="warranties">
		<h2>13. Disclaimers</h2>
		<p>
			Except where prohibited by mandatory law, the Service is provided on an "as is" and "as
			available" basis without warranties of any kind, express or implied, including warranties of
			merchantability, fitness for a particular purpose, non-infringement, or uninterrupted operation.
		</p>
	</section>

	<section id="liability">
		<h2>14. Limitation of liability</h2>
		<p>
			To the maximum extent permitted by law, neither party will be liable for any indirect,
			incidental, special, consequential, or punitive damages, or for loss of profits, data, or
			goodwill. Because the hosted Service is provided free of charge, our total aggregate liability
			arising out of or relating to these Terms is limited to one hundred pounds sterling (£100).
		</p>
		<p>Nothing in these Terms limits liability for fraud, gross negligence, or any liability that cannot lawfully be limited.</p>
	</section>

	<section id="indemnity">
		<h2>15. Indemnification</h2>
		<p>
			You agree to defend, indemnify, and hold harmless Mindroot Ltd and its officers, employees, and
			affiliates from any claims, damages, liabilities, and reasonable legal costs arising from your
			Configuration, your use of the Service in breach of these Terms, or your violation of any law or
			third-party right.
		</p>
	</section>

	<section id="termination">
		<h2>16. Termination</h2>
		<p>
			You may stop using the Service at any time by removing the bot from your server. We may suspend
			or terminate access if you breach these Terms, if your use creates a security risk to the Service
			or other communities, or if we are required to do so by law or by Discord. On termination, your
			right to use the hosted Service ends, and data is handled as described in our
			<a href="/privacy">Privacy Policy</a>.
		</p>
	</section>

	<section id="law">
		<h2>17. Governing law</h2>
		<p>
			These Terms are governed by the laws of England and Wales. You and Mindroot Ltd submit to the
			exclusive jurisdiction of the courts of London, United Kingdom, for any dispute arising out of or
			relating to these Terms or your use of the Service.
		</p>
	</section>

	<section id="changes">
		<h2>18. Changes to these terms</h2>
		<p>
			We may update these Terms from time to time. The "Last updated" date above reflects the most
			recent change. If we make material changes, we will provide reasonable notice through the
			dashboard or our repository before they take effect. Continued use after the effective date
			constitutes acceptance of the updated Terms.
		</p>
	</section>

	<section id="contact">
		<h2>19. Contact</h2>
		<p>
			Questions about these Terms can be sent to <a href="mailto:hello@dia.xyz">hello@dia.xyz</a> or by
			post to Mindroot Ltd, 71–75 Shelton Street, London, England, WC2H 9JQ.
		</p>
	</section>
</LegalDoc>
