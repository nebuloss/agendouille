package i18n

// The family web app's pages: titles, navigation, signing in, enrollment
// links and approvals (internal/web/assets/templates/app).
func init() {
	register(map[string]map[Lang]string{
		"app.title.davx5": {FR: "Configurer DAVx5", EN: "Set up DAVx5"},
		"app.davx5.title": {FR: "Configurer DAVx5", EN: "Set up DAVx5"},
		"app.davx5.step1": {
			FR: "Installez DAVx5 sur ce téléphone, si ce n’est pas déjà fait :",
			EN: "Install DAVx5 on this phone, if it is not there yet:",
		},
		"app.davx5.step2": {
			FR: "Touchez « Ouvrir dans DAVx5 » : le compte s’y remplit tout seul.",
			EN: "Tap ‘Open in DAVx5’: the account fills itself in.",
		},
		"app.davx5.step3": {
			FR: "Dans DAVx5, validez la connexion, puis choisissez les calendriers à synchroniser.",
			EN: "In DAVx5, confirm the sign-in, then choose the calendars to sync.",
		},
		"app.davx5.open":   {FR: "Ouvrir dans DAVx5", EN: "Open in DAVx5"},
		"app.davx5.manual": {FR: "Saisir le compte à la main", EN: "Enter the account by hand"},
		"app.davx5.manualHelp": {
			FR: "Dans DAVx5, ajoutez un compte avec « Se connecter avec une URL et un nom d’utilisateur », puis saisissez :",
			EN: "In DAVx5, add an account with ‘Login with URL and user name’, then enter:",
		},
		"app.davx5.server":   {FR: "Adresse", EN: "Base URL"},
		"app.davx5.username": {FR: "Utilisateur", EN: "User name"},
		"app.davx5.password": {FR: "Mot de passe", EN: "Password"},
		"app.davx5.expires": {
			FR: "Cette page ne fonctionne que dix minutes après l’ajout de l’appareil. Passé ce délai, retirez l’appareil et ajoutez-le à nouveau.",
			EN: "This page only works for ten minutes after the device was added. After that, remove the device and add it again.",
		},
		"app.davx5.gone": {
			FR: "Ce lien a expiré. Dans agendouille, retirez l’appareil, puis ajoutez-le à nouveau pour obtenir un nouveau code.",
			EN: "This link has expired. In agendouille, remove the device, then add it again to get a new code.",
		},
		"app.enroll.androidTitle": {
			FR: "Sur un téléphone Android",
			EN: "On an Android phone",
		},
		"app.enroll.androidHelp": {
			FR: "L’application agendouille synchronise vos calendriers avec ceux du téléphone. Installez-la, puis ouvrez ce lien avec elle.",
			EN: "The agendouille app syncs your calendars with the phone’s. Install it, then open this link with it.",
		},
		"app.enroll.openApp": {
			FR: "Ouvrir dans l’application",
			EN: "Open in the app",
		},
		"app.signin.checksum": {FR: "Empreinte du fichier", EN: "File checksum"},
		"app.signin.getAppHelp": {
			FR: "Sur un téléphone Android, l’application agendouille synchronise vos calendriers avec ceux du téléphone.",
			EN: "On an Android phone, the agendouille app syncs your calendars with the phone’s.",
		},
		"app.enroll.getApp": {
			FR: "Télécharger l’application",
			EN: "Download the app",
		},
		"app.enroll.orBrowser": {
			FR: "Ou continuez dans ce navigateur :",
			EN: "Or continue in this browser:",
		},
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
			FR: "Dans l’application agendouille, onglet Appareils, scannez ce code, puis confirmez sur le téléphone. Sans appareil photo, saisissez le code ci-dessous.",
			EN: "In the agendouille app, Devices tab, scan this code, then confirm on the phone. Without a camera, type the code below.",
		},
		"app.signin.scanned": {
			FR: "{name}, confirmez sur votre téléphone.",
			EN: "{name}, confirm on your phone.",
		},
		"app.signin.qrAlt":    {FR: "QR code de connexion", EN: "Sign-in QR code"},
		"app.signin.code":     {FR: "Code", EN: "Code"},
		"app.signin.valid":    {FR: "Ce code est valable deux minutes.", EN: "This code is valid for two minutes."},
		"app.signin.auto":     {FR: "Cette page s’ouvrira d’elle-même une fois la connexion approuvée.", EN: "This page will continue on its own once the sign-in is approved."},
		"app.signin.newCode":  {FR: "Obtenir un nouveau code", EN: "Get a new code"},
		"app.signin.continue": {FR: "Continuer après l’approbation", EN: "Continue after approval"},

		// An enrollment link opened in a browser.
		"app.enroll.title": {FR: "Bienvenue", EN: "Welcome"},
		"app.enroll.intro": {
			FR: "Ce lien de connexion relie cet appareil aux calendriers de la famille. Il ne peut servir qu’une seule fois.",
			EN: "This sign-in link connects this device to the family’s calendars. It can be used only once.",
		},
		"app.enroll.askNew": {
			FR: "Demandez un nouveau lien de connexion à l’administrateur.",
			EN: "Ask the administrator for a new sign-in link.",
		},
		"app.enroll.continue": {FR: "Continuer", EN: "Continue"},

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
		"app.approve.warning": {
			FR: "Ne confirmez que si ce navigateur est le vôtre et que vous l’avez devant vous. Ne scannez jamais un QR code que quelqu’un vous a envoyé.",
			EN: "Only confirm if this browser is yours and in front of you. Never scan a QR code someone sent you.",
		},
		"app.approve.confirm": {FR: "Se connecter", EN: "Sign in"},
		"app.approve.refused": {
			FR: "Connexion refusée. L’autre appareil devra recommencer.",
			EN: "Sign-in refused. The other device will have to start again.",
		},
		"app.approve.notMe":     {FR: "Ce n’est pas moi", EN: "This wasn’t me"},
		"app.approve.codeLabel": {FR: "Code affiché sur l’autre écran", EN: "Code shown on the other screen"},
		"app.approve.next":      {FR: "Suivant", EN: "Next"},
	})
}
