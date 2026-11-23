package i18n

// The settings' help texts, shown under each key on the admin Réglages
// page (GET /api/admin/settings carries them in the reader's language).
// One key per setting: "setting.<key>.help".
func init() {
	register(map[string]map[Lang]string{
		"setting.public_listen.help": {
			FR: "Adresse sur laquelle écoutent CalDAV et l’application (celle vers laquelle renvoie le proxy de la DMZ), par exemple 0.0.0.0:8791.",
			EN: "Address CalDAV and the app listen on (the one the DMZ proxy forwards to), for example 0.0.0.0:8791.",
		},
		"setting.public_url.help": {
			FR: "Adresse publique en https, telle que le proxy la sert. Elle figure dans chaque QR de connexion et dans chaque téléphone configuré.",
			EN: "Public https address, as the proxy serves it. It appears in every sign-in QR code and on every phone set up.",
		},
		"setting.public_allow_insecure.help": {
			FR: "Accepter une adresse en http:// pour public_url (uniquement pour des essais sur le réseau local).",
			EN: "Accept an http:// address for public_url (only for tests on the local network).",
		},
		"setting.trusted_proxies.help": {
			FR: "Adresses ou plages CIDR du proxy : leur en-tête X-Forwarded-For est pris en compte.",
			EN: "Addresses or CIDR ranges of the proxy: their X-Forwarded-For header is trusted.",
		},
		"setting.backup_schedule.help": {
			FR: "Heure de la sauvegarde quotidienne (HH:MM, heure du serveur).",
			EN: "Time of the daily backup (HH:MM, server time).",
		},
		"setting.backup_keep.help": {
			FR: "Nombre de sauvegardes conservées ; au-delà, les plus anciennes sont supprimées.",
			EN: "Number of backups kept; beyond that, the oldest are deleted.",
		},
		"setting.sync_token_retention_days.help": {
			FR: "Durée de conservation, en jours, de l’historique des modifications. Un appareil resté hors ligne plus longtemps resynchronise tout son agenda.",
			EN: "How many days the change history is kept. A device offline for longer syncs its whole calendar again.",
		},
		"setting.max_object_bytes.help": {
			FR: "Taille maximale d’un événement, en octets.",
			EN: "Maximum size of one event, in bytes.",
		},
		"setting.max_calendars_per_user.help": {
			FR: "Nombre maximal de calendriers par utilisateur.",
			EN: "Maximum number of calendars per user.",
		},
		"setting.max_events_per_calendar.help": {
			FR: "Nombre maximal d’événements par calendrier.",
			EN: "Maximum number of events per calendar.",
		},
		"setting.max_devices_per_user.help": {
			FR: "Nombre maximal d’appareils synchronisés par utilisateur.",
			EN: "Maximum number of synced devices per user.",
		},
		"setting.family_timezone.help": {
			FR: "Fuseau horaire de la famille, par exemple Europe/Paris : toutes les heures de l’agenda y sont exprimées.",
			EN: "The family's time zone, for example Europe/Paris: every time in the calendar is given in it.",
		},
		"setting.palette.help": {
			FR: "Couleurs attribuées aux nouveaux utilisateurs et calendriers, en hexadécimal.",
			EN: "Colours given to new users and calendars, in hexadecimal.",
		},
		"setting.enrolment_link_ttl_hours.help": {
			FR: "Durée de validité, en heures, d’un lien de connexion.",
			EN: "How long a sign-in link stays valid, in hours.",
		},
		"setting.session_ttl_days.help": {
			FR: "Durée, en jours, pendant laquelle un navigateur reste connecté.",
			EN: "How many days a browser stays signed in.",
		},
		"setting.app_sync_interval_minutes.help": {
			FR: "Intervalle, en minutes, entre deux synchronisations de l’application Android (15 au minimum).",
			EN: "Minutes between two syncs of the Android app (15 at least).",
		},
	})
}
