package i18n

// The admin UI's own words on the server: its shell (the top bar, the
// rail, the trail), its sign-in page, and the labels of its navigation.
func init() {
	register(map[string]map[Lang]string{
		"admin.title": {
			FR: "Administration",
			EN: "Administration",
		},

		// The navigation (internal/web/admin/nav.go): the sections, then an
		// item's pages.
		"admin.nav.overview": {
			FR: "Vue d’ensemble",
			EN: "Overview",
		},
		"admin.nav.users": {
			FR: "Utilisateurs",
			EN: "Users",
		},
		"admin.nav.groups": {
			FR: "Groupes",
			EN: "Groups",
		},
		"admin.nav.calendars": {
			FR: "Calendriers",
			EN: "Calendars",
		},
		"admin.nav.activity": {
			FR: "Activité",
			EN: "Activity",
		},
		"admin.nav.backups": {
			FR: "Sauvegardes",
			EN: "Backups",
		},
		"admin.nav.settings": {
			FR: "Réglages",
			EN: "Settings",
		},
		"admin.nav.devices": {
			FR: "Appareils",
			EN: "Devices",
		},
		"admin.nav.members": {
			FR: "Membres",
			EN: "Members",
		},
		// An item's page title: the page, then the item ("Appareils · Léa").
		"admin.nav.pageOf": {
			FR: "{name} · {page}",
			EN: "{name} · {page}",
		},

		// The shell (templates/admin/shell.html).
		"admin.shell.home": {
			FR: "agendouille · administration",
			EN: "agendouille · administration",
		},
		"admin.shell.tagline": {
			FR: "administration",
			EN: "administration",
		},
		"admin.shell.signout": {
			FR: "Se déconnecter",
			EN: "Sign out",
		},
		"admin.shell.trail": {
			FR: "Fil d’Ariane",
			EN: "Breadcrumb",
		},
		"admin.shell.fold": {
			FR: "Réduire le menu",
			EN: "Collapse the menu",
		},
		"admin.shell.unfold": {
			FR: "Déplier le menu",
			EN: "Expand the menu",
		},
		"admin.shell.pages": {
			FR: "Pages",
			EN: "Pages",
		},
		"admin.shell.sections": {
			FR: "Sections",
			EN: "Sections",
		},
		"admin.shell.noscript": {
			FR: "L’administration a besoin de JavaScript pour fonctionner. Activez-le dans votre navigateur, puis rechargez la page.",
			EN: "The administration needs JavaScript to work. Turn it on in your browser, then reload the page.",
		},

		// The sign-in page (templates/admin/signin.html).
		"admin.signin.password": {
			FR: "Mot de passe administrateur",
			EN: "Administrator password",
		},
		"admin.signin.submit": {
			FR: "Se connecter",
			EN: "Sign in",
		},
		"admin.signin.wrongPassword": {
			FR: "Mot de passe incorrect. Vérifiez-le, puis réessayez.",
			EN: "Incorrect password. Check it, then try again.",
		},
		"admin.signin.failed": {
			FR: "La connexion a échoué. Réessayez dans quelques instants.",
			EN: "Signing in failed. Try again in a few moments.",
		},
	})
}
