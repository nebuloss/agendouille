package i18n

// The messages of API errors a person reads (the web app's and the admin
// UI's toasts and dialogs, and the error lines of the sign-in pages). The
// error codes beside them are for programs and are never translated
// (CONTRACTS.md 3.1); the services' own error texts stay English, for the
// log and the CLI. Each message says what happened, then what to do.
func init() {
	register(map[string]map[Lang]string{
		// Any route.
		"error.internal": {
			FR: "Une erreur inattendue s’est produite. Réessayez dans un instant.",
			EN: "Something went wrong. Try again in a moment.",
		},
		"error.request.invalid": {
			FR: "La demande n’a pas pu être lue. Rechargez la page, puis réessayez.",
			EN: "The request could not be read. Reload the page and try again.",
		},
		"error.request.tooLarge": {
			FR: "Le texte envoyé est trop long. Raccourcissez-le, puis réessayez.",
			EN: "What you sent is too long. Shorten it and try again.",
		},
		"error.request.crossOrigin": {
			FR: "Cette demande ne vient pas d’une page de ce site. Rechargez la page, puis réessayez.",
			EN: "This request did not come from one of this site’s pages. Reload the page and try again.",
		},
		"error.notFound": {
			FR: "Cet élément est introuvable : il a peut-être été supprimé. Rechargez la page.",
			EN: "This item cannot be found: it may have been deleted. Reload the page.",
		},

		// Signing in.
		"error.signin.required": {
			FR: "Votre session a expiré. Reconnectez-vous pour continuer.",
			EN: "Your session has ended. Sign in again to continue.",
		},
		"error.signin.busy": {
			FR: "Trop de connexions sont en attente. Réessayez dans une minute.",
			EN: "Too many sign-ins are waiting. Try again in a minute.",
		},
		"error.enroll.used": {
			FR: "Ce lien de connexion a déjà été utilisé. Demandez-en un nouveau.",
			EN: "This sign-in link has already been used. Ask for a new one.",
		},
		"error.enroll.expired": {
			FR: "Ce lien de connexion a expiré. Demandez-en un nouveau.",
			EN: "This sign-in link has expired. Ask for a new one.",
		},
		"error.enroll.invalid": {
			FR: "Ce lien de connexion n’est pas valable. Vérifiez qu’il est complet, ou demandez-en un nouveau.",
			EN: "This sign-in link is not valid. Check that it is complete, or ask for a new one.",
		},
		"error.approval.refused": {
			FR: "La connexion a été refusée sur l’autre appareil. Recommencez si c’était bien vous.",
			EN: "The sign-in was refused on the other device. Start again if it was you.",
		},
		"error.approval.expired": {
			FR: "Ce code a expiré. Recommencez la connexion sur l’autre appareil.",
			EN: "This code has expired. Start signing in again on the other device.",
		},
		"error.approval.used": {
			FR: "Cette demande a déjà été approuvée.",
			EN: "This request has already been approved.",
		},
		"error.approval.notFound": {
			FR: "Ce code ne correspond à aucune demande en attente. Vérifiez-le, puis réessayez.",
			EN: "This code does not match any waiting request. Check it and try again.",
		},
		"error.device.unknown": {
			FR: "Cet appareil n’est pas reconnu. Reconnectez l’application à agendouille.",
			EN: "This device is not recognised. Connect the app to agendouille again.",
		},
		"error.app.sessionOnly": {
			FR: "Seule l’application agendouille peut ouvrir une session avec cet appareil.",
			EN: "Only the agendouille app can open a session with this device.",
		},

		// Calendars and events.
		"error.calendar.nameRequired": {
			FR: "Donnez un nom au calendrier.",
			EN: "Give the calendar a name.",
		},
		"error.calendar.badName": {
			FR: "Ce nom de calendrier n’est pas valable. Choisissez-en un autre.",
			EN: "This calendar name is not valid. Choose another one.",
		},
		"error.calendar.exists": {
			FR: "Cette personne a déjà un calendrier de ce nom. Renommez le calendrier, puis réessayez.",
			EN: "This person already has a calendar with this name. Rename the calendar and try again.",
		},
		"error.calendar.notOwner": {
			FR: "Seul le propriétaire de ce calendrier peut faire cela.",
			EN: "Only this calendar’s owner can do this.",
		},
		"error.calendar.readOnly": {
			FR: "Ce calendrier est en lecture seule pour vous. Demandez à son propriétaire l’accès en lecture et écriture.",
			EN: "This calendar is read-only for you. Ask its owner for read and write access.",
		},
		"error.event.uidConflict": {
			FR: "Cet événement existe déjà dans ce calendrier.",
			EN: "This event is already in this calendar.",
		},
		"error.event.stale": {
			FR: "Cet événement a été modifié entre-temps. Rouvrez-le pour voir la dernière version, puis refaites vos changements.",
			EN: "This event was changed in the meantime. Open it again to see the latest version, then make your changes again.",
		},
		"error.event.versionMissing": {
			FR: "La version de l’événement manque dans la demande. Rechargez la page, puis réessayez.",
			EN: "The request does not say which version of the event it changes. Reload the page and try again.",
		},
		"error.event.invalid": {
			FR: "Cet événement n’a pas pu être enregistré : ses informations ne sont pas valables. Vérifiez les dates et la répétition, puis réessayez.",
			EN: "This event could not be saved because its details are not valid. Check the dates and the repeat, then try again.",
		},
		"error.event.endsBeforeStart": {
			FR: "L’événement se termine avant de commencer. Corrigez les dates, puis réessayez.",
			EN: "The event ends before it starts. Correct the dates and try again.",
		},
		"error.event.fieldTooLong": {
			FR: "Le titre, le lieu ou la description est trop long. Raccourcissez-le, puis réessayez.",
			EN: "The title, place or description is too long. Shorten it and try again.",
		},
		"error.event.unsupported": {
			FR: "Seuls les événements sont pris en charge, pas les tâches ni les notes.",
			EN: "Only events are supported, not tasks or notes.",
		},
		"error.event.tooLarge": {
			FR: "Cet événement est trop volumineux pour être enregistré. Raccourcissez sa description, puis réessayez.",
			EN: "This event is too large to save. Shorten its description and try again.",
		},
		"error.occurrences.range": {
			FR: "La période demandée n’est pas valable : elle compte au plus {days} jours. Rechargez la page.",
			EN: "The requested period is not valid: it can be at most {days} days. Reload the page.",
		},
		"error.import.unreadable": {
			FR: "Le fichier n’a pas pu être lu. Réessayez.",
			EN: "The file could not be read. Try again.",
		},
		"error.import.invalid": {
			FR: "Ce fichier n’est pas un calendrier valable. Choisissez un fichier .ics exporté d’une autre application d’agenda.",
			EN: "This file is not a valid calendar. Choose an .ics file exported from another calendar app.",
		},

		// Sharing.
		"error.share.notVisible": {
			FR: "Vous ne pouvez partager ce calendrier qu’avec les personnes de vos groupes.",
			EN: "You can only share this calendar with people in your groups.",
		},
		"error.share.owner": {
			FR: "Le propriétaire a déjà accès à son calendrier.",
			EN: "The owner already has access to their calendar.",
		},
		"error.share.invalid": {
			FR: "Ce partage n’est pas valable. Rechargez la page, puis réessayez.",
			EN: "This share is not valid. Reload the page and try again.",
		},

		// Limits.
		"error.quota.calendars": {
			FR: "Le nombre maximal de calendriers par personne ({max}) est atteint. Supprimez-en un pour en créer un autre.",
			EN: "The maximum number of calendars per person ({max}) has been reached. Delete one to create another.",
		},
		"error.quota.events": {
			FR: "Ce calendrier a atteint le nombre maximal d’événements ({max}). Supprimez-en pour en ajouter d’autres.",
			EN: "This calendar has reached the maximum number of events ({max}). Delete some to add others.",
		},
		"error.quota.devices": {
			FR: "Le nombre maximal d’appareils par personne ({max}) est atteint. Retirez-en un pour en ajouter un autre.",
			EN: "The maximum number of devices per person ({max}) has been reached. Remove one to add another.",
		},
		"error.quota": {
			FR: "Une limite a été atteinte. Supprimez des éléments, puis réessayez.",
			EN: "A limit has been reached. Delete some items and try again.",
		},

		// People, devices, links.
		"error.name.invalid": {
			FR: "Ce nom n’est pas valable : il commence par une lettre minuscule, suivie d’au plus 31 lettres minuscules, chiffres ou tirets, et ne doit pas être réservé (admin, cli, root, system).",
			EN: "This name is not valid: it starts with a lowercase letter, followed by up to 31 lowercase letters, digits or dashes, and must not be reserved (admin, cli, root, system).",
		},
		"error.name.taken": {
			FR: "Ce nom est déjà pris, ou l’a été : un nom n’est jamais réutilisé. Choisissez-en un autre.",
			EN: "This name is taken, or once was: names are never reused. Choose another one.",
		},
		"error.displayName.invalid": {
			FR: "Le nom affiché doit compter de 1 à 64 caractères, sur une seule ligne.",
			EN: "The display name must be 1 to 64 characters, on one line.",
		},
		"error.color.invalid": {
			FR: "Cette couleur n’est pas valable. Choisissez une couleur de la forme #3b6ea5.",
			EN: "This colour is not valid. Choose a colour written like #3b6ea5.",
		},
		"error.device.label": {
			FR: "Le nom de l’appareil doit compter de 1 à 100 caractères.",
			EN: "The device name must be 1 to 100 characters.",
		},
		"error.device.invalid": {
			FR: "Cet appareil n’a pas pu être ajouté. Rechargez la page, puis réessayez.",
			EN: "This device could not be added. Reload the page and try again.",
		},
		"error.certificate.linkGone": {
			FR: "Ce lien a déjà été utilisé ou a expiré. Demandez un nouveau certificat pour cet appareil.",
			EN: "This link has already been used or has expired. Ask for a new certificate for this device.",
		},
		"error.profile.linkGone": {
			FR: "Ce lien de profil a déjà été utilisé ou a expiré. Ajoutez l’appareil à nouveau.",
			EN: "This profile link has already been used or has expired. Add the device again.",
		},
		"error.link.label": {
			FR: "Le nom du lien doit compter de 1 à 100 caractères.",
			EN: "The link name must be 1 to 100 characters.",
		},
		"error.link.noCalendar": {
			FR: "Choisissez au moins un calendrier pour ce lien.",
			EN: "Choose at least one calendar for this link.",
		},

		// The admin UI.
		"error.admin.signinRequired": {
			FR: "Votre session d’administration a expiré. Reconnectez-vous pour continuer.",
			EN: "Your administration session has ended. Sign in again to continue.",
		},
		"error.admin.noUser": {
			FR: "Cet utilisateur est introuvable : il a peut-être été supprimé. Rechargez la page.",
			EN: "This user cannot be found: they may have been removed. Reload the page.",
		},
		"error.admin.nobody": {
			FR: "Cet utilisateur ou ce groupe est introuvable. Vérifiez le nom, puis réessayez.",
			EN: "This user or group cannot be found. Check the name and try again.",
		},
		"error.admin.noPublicURL": {
			FR: "Aucune adresse publique n’est configurée : le lien de connexion ne peut pas être créé. Renseignez public_url dans le fichier de configuration.",
			EN: "No public address is set, so the sign-in link cannot be made. Set public_url in the configuration file.",
		},
		"error.admin.enrollmentUsed": {
			FR: "Ce lien de connexion a déjà été utilisé.",
			EN: "This sign-in link has already been used.",
		},
		"error.admin.enrollmentExpired": {
			FR: "Ce lien de connexion a expiré. Créez-en un nouveau.",
			EN: "This sign-in link has expired. Create a new one.",
		},
		"error.admin.ownsCalendars": {
			FR: "Pour chaque calendrier de cet utilisateur, choisissez à qui le transférer ou s’il doit être supprimé.",
			EN: "For each of this user’s calendars, choose who receives it or whether it is deleted.",
		},
		"error.admin.groupHasShares": {
			FR: "Des calendriers sont partagés avec ce groupe. Le supprimer retirera ces partages.",
			EN: "Calendars are shared with this group. Deleting it will remove those shares.",
		},
		"error.admin.confirmCalendar": {
			FR: "Saisissez le nom du calendrier pour confirmer sa suppression.",
			EN: "Type the calendar’s name to confirm its deletion.",
		},
		"error.admin.badBefore": {
			FR: "La date de début de la liste n’est pas valable. Rechargez la page.",
			EN: "The list’s starting date is not valid. Reload the page.",
		},
		"error.admin.davAuthMode": {
			FR: "Ce mode de connexion n’est pas valable. Choisissez l’un des modes proposés.",
			EN: "This sign-in mode is not valid. Choose one of the modes offered.",
		},
		"error.admin.confirmRestore": {
			FR: "Saisissez le nom du fichier de la sauvegarde pour confirmer la restauration.",
			EN: "Type the backup file’s name to confirm the restore.",
		},
		"error.admin.backupDamaged": {
			FR: "Cette sauvegarde est endommagée ou illisible : elle n’a pas été restaurée. Choisissez-en une autre.",
			EN: "This backup is damaged or unreadable, so it was not restored. Choose another one.",
		},
		"error.admin.restarting": {
			FR: "Le serveur redémarre avec cette sauvegarde. Reconnectez-vous dans quelques secondes.",
			EN: "The server is restarting with this backup. Sign in again in a few seconds.",
		},
		"error.backup.notFound": {
			FR: "Cette sauvegarde est introuvable. Rechargez la page.",
			EN: "This backup cannot be found. Reload the page.",
		},
		"error.setting.unknown": {
			FR: "Ce réglage n’existe pas.",
			EN: "This setting does not exist.",
		},

		// The overview's warnings (also the admin's server notices).
		"error.warning.noPublicListener": {
			FR: "CalDAV et l’application familiale sont arrêtés. Renseignez public_listen et public_url dans le fichier de configuration.",
			EN: "CalDAV and the family app are stopped. Set public_listen and public_url in the configuration file.",
		},
		"error.warning.publicDown": {
			FR: "L’accès public n’a pas pu démarrer sur {address} : {error}. Vérifiez cette adresse dans le fichier de configuration.",
			EN: "Public access could not start on {address}: {error}. Check this address in the configuration file.",
		},
		"error.warning.publicStopped": {
			FR: "L’accès public ne fonctionne pas.",
			EN: "Public access is not running.",
		},
		"error.warning.adminInsecure": {
			FR: "L’administration est servie en HTTP non chiffré au-delà de cette machine. Gardez-la sur le réseau local.",
			EN: "The administration is served over unencrypted HTTP beyond this machine. Keep it on the local network.",
		},
		"error.warning.backupOld": {
			FR: "Aucune sauvegarde depuis deux jours.",
			EN: "No backup in the last two days.",
		},
	})
}
