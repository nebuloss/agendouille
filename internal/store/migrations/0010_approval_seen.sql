-- Who looked a browser's sign-in up (scanned its QR code or typed its
-- code), so the browser can say « Léa, confirmez sur votre téléphone »
-- while the phone asks to confirm (DECISIONS 2026-11-24: scan and confirm,
-- no number to pick; the number column stays, unused).
ALTER TABLE approvals ADD COLUMN seen_by TEXT;
