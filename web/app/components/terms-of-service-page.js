const tmpl = document.createElement("template")
tmpl.innerHTML = `
    <main class="container terms-of-service-page">
        <h1>Terms of Service</h1>
        <p>These Terms of Service govern your use of Nakama and any related services we provide.</p>
        <p>By accessing or using Nakama, you agree to these terms. If you do not agree, do not use the service.</p>

        <h3>Use of the Service</h3>
        <p>You may use Nakama only in compliance with applicable law and these terms.</p>
        <p>You are responsible for the activity that occurs through your account and for maintaining the security of your access credentials.</p>

        <h3>Acceptable Conduct</h3>
        <p>You agree not to misuse the service. This includes, without limitation, attempting to interfere with the normal operation of Nakama, accessing data you are not authorized to access, or using the service for unlawful, abusive, or fraudulent purposes.</p>
        <p>You must not upload or share content that violates the law or the rights of others.</p>

        <h3>User Content</h3>
        <p>You retain ownership of the content you submit to Nakama.</p>
        <p>By posting content, you grant us a non-exclusive license to host, store, process, and display that content only as needed to operate and improve the service.</p>
        <p>You are solely responsible for the content you publish and for ensuring that you have the necessary rights to publish it.</p>

        <h3>Accounts</h3>
        <p>We may suspend or terminate access to accounts that violate these terms or create risk for the service, other users, or us.</p>
        <p>You may stop using the service at any time.</p>

        <h3>Availability</h3>
        <p>We may change, suspend, or discontinue any part of Nakama at any time, with or without notice.</p>
        <p>We do not guarantee that the service will always be available, uninterrupted, secure, or error-free.</p>

        <h3>Disclaimer</h3>
        <p>Nakama is provided on an “as is” and “as available” basis to the extent permitted by law, without warranties of any kind, whether express or implied.</p>

        <h3>Limitation of Liability</h3>
        <p>To the maximum extent permitted by law, we are not liable for any indirect, incidental, special, consequential, or punitive damages, or any loss of data, profits, goodwill, or business opportunities arising from your use of the service.</p>

        <h3>Changes to These Terms</h3>
        <p>We may update these terms from time to time. If we make material changes, we may provide notice through the service or by other reasonable means.</p>
        <p>Your continued use of Nakama after the updated terms take effect means you accept the revised terms.</p>

        <h3>Contact</h3>
        <p>If you have questions about these terms, you can contact us at <a href="mailto:contact@nakama.social">contact@nakama.social</a>.</p>
    </main>
`

export default function TermsOfService() {
    return tmpl.content.cloneNode(true)
}
