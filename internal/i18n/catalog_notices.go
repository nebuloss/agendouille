package i18n

// The admin's notification centre (internal/web/admin/notices.go), and the
// header row of the activity log's CSV export, which the admin reads in a
// spreadsheet. The rows themselves stay data: action codes, user names.
func init() {
	register(map[string]map[Lang]string{
		"notice.devices_without_certificate.title.one": {
			FR: "{count} appareil sans certificat ne se synchronise plus",
			EN: "{count} device without a certificate no longer syncs",
		},
		"notice.devices_without_certificate.title.other": {
			FR: "{count} appareils sans certificat ne se synchronisent plus",
			EN: "{count} devices without a certificate no longer sync",
		},
		"notice.devices_without_certificate.detail": {
			FR: "La connexion CalDAV exige désormais un certificat. Chaque utilisateur peut en obtenir un depuis la page Appareils de l’application.",
			EN: "CalDAV sign-in now requires a certificate. Each user can get one from the Devices page in the app.",
		},
		"notice.device.stale.title": {
			FR: "« {device} » ne s’est pas synchronisé depuis une semaine",
			EN: "“{device}” has not synced for a week",
		},
		"notice.device.never.title": {
			FR: "« {device} » ne s’est jamais synchronisé",
			EN: "“{device}” has never synced",
		},
		"notice.device.detail": {
			FR: "Appareil de {user}. Vérifiez qu’il est allumé et que son mot de passe est correct, ou retirez-le.",
			EN: "Belongs to {user}. Check that it is switched on and that its password is right, or remove it.",
		},
		"notice.user.never.title": {
			FR: "Aucune connexion de {name} pour le moment",
			EN: "{name} has never signed in",
		},
		"notice.user.never.detail": {
			FR: "Donnez-lui un QR de connexion depuis sa page.",
			EN: "Give them a sign-in QR code from their page.",
		},

		"activity.csv.at":       {FR: "date", EN: "time"},
		"activity.csv.by":       {FR: "auteur", EN: "by"},
		"activity.csv.kind":     {FR: "type", EN: "kind"},
		"activity.csv.action":   {FR: "action", EN: "action"},
		"activity.csv.calendar": {FR: "calendrier", EN: "calendar"},
		"activity.csv.via":      {FR: "via", EN: "via"},
		"activity.csv.device":   {FR: "appareil", EN: "device"},
		"activity.csv.detail":   {FR: "détail", EN: "detail"},
	})
}
