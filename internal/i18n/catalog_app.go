package i18n

// The family web app's pages: titles, navigation, signing in, enrolment
// links and approvals (internal/web/assets/templates/app).
func init() {
	register(map[string]map[Lang]string{
		// Titles, in the header and the browser's tab.
		"app.title.calendar":  {FR: "Calendrier", EN: "Calendar"},
		"app.title.calendars": {FR: "Calendriers", EN: "Calendars"},
		"app.title.devices":   {FR: "Appareils", EN: "Devices"},
		"app.title.me":        {FR: "Moi", EN: "Me"},
		"app.title.signin":    {FR: "Se connecter", EN: "Sign in"},
		"app.title.welcome":   {FR: "Bienvenue", EN: "Welcome"},
		"app.title.approve":   {FR: "Approuver une connexion", EN: "Approve a sign-in"},

		// The shell.
		"app.nav.calendar":  {FR: "Calendrier", EN: "Calendar"},
		"app.nav.calendars": {FR: "Calendriers", EN: "Calendars"},
		"app.nav.devices":   {FR: "Appareils", EN: "Devices"},
		"app.nav.me":        {FR: "Moi", EN: "Me"},
		"app.signout":       {FR: "Se déconnecter", EN: "Sign out"},
		"app.noscript": {
			FR: "agendouille a besoin de JavaScript pour afficher le calendrier.",
			EN: "agendouille needs JavaScript to show the calendar.",
		},

		// Signing in: the code another device approves.
		"app.signin.title": {FR: "Se connecter", EN: "Sign in"},
		"app.signin.how": {
			FR: "Sur un appareil déjà connecté, scannez ce code avec l’appareil photo, ou ouvrez «\u00a0Approuver une connexion\u00a0» et saisissez le code ci-dessous.",
			EN: "On a device that is already signed in, scan this code with the camera, or open “Approve a sign-in” and enter the code below.",
		},
		"app.signin.qrAlt":    {FR: "QR code de connexion", EN: "Sign-in QR code"},
		"app.signin.code":     {FR: "Code", EN: "Code"},
		"app.signin.number":   {FR: "Nombre à choisir", EN: "Number to choose"},
		"app.signin.valid":    {FR: "Ce code est valable deux minutes.", EN: "This code is valid for two minutes."},
		"app.signin.auto":     {FR: "Cette page s’ouvrira d’elle-même une fois la connexion approuvée.", EN: "This page will continue on its own once the sign-in is approved."},
		"app.signin.newCode":  {FR: "Obtenir un nouveau code", EN: "Get a new code"},
		"app.signin.continue": {FR: "Continuer après l’approbation", EN: "Continue after approval"},

		// An enrolment link opened in a browser.
		"app.enrol.title": {FR: "Bienvenue", EN: "Welcome"},
		"app.enrol.intro": {
			FR: "Ce lien de connexion relie cet appareil aux calendriers de la famille. Il ne peut servir qu’une seule fois.",
			EN: "This sign-in link connects this device to the family’s calendars. It can be used only once.",
		},
		"app.enrol.askNew": {
			FR: "Demandez un nouveau lien de connexion à l’administrateur.",
			EN: "Ask the administrator for a new sign-in link.",
		},
		"app.enrol.continue": {FR: "Continuer", EN: "Continue"},

		// Approving a sign-in from a signed-in device.
		"app.approve.title": {FR: "Approuver une connexion", EN: "Approve a sign-in"},
		"app.approve.done": {
			FR: "Connexion approuvée. L’autre appareil est maintenant connecté.",
			EN: "Sign-in approved. The other device is now signed in.",
		},
		"app.approve.back": {FR: "Revenir au calendrier", EN: "Back to the calendar"},
		"app.approve.question": {
			FR: "Connecter «\u00a0{device}\u00a0», depuis l’adresse {ip}, en tant que {user}\u00a0?",
			EN: "Sign in “{device}”, from {ip}, as {user}?",
		},
		"app.approve.pick": {
			FR: "Touchez le nombre affiché sur l’autre écran. Un mauvais choix annule la demande.",
			EN: "Tap the number shown on the other screen. A wrong choice cancels the request.",
		},
		"app.approve.notMe":     {FR: "Ce n’est pas moi", EN: "This wasn’t me"},
		"app.approve.codeLabel": {FR: "Code affiché sur l’autre écran", EN: "Code shown on the other screen"},
		"app.approve.next":      {FR: "Suivant", EN: "Next"},
	})
}
