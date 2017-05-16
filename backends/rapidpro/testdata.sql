/* Org with id 100 */
DELETE FROM orgs_org;
INSERT INTO orgs_org("id", "name", "language")
              VALUES(100, 'Test Org', 'eng');

/* Channel with id 101 */
DELETE FROM channels_channel;
INSERT INTO channels_channel("id", "is_active", "created_on", "modified_on", "uuid", "channel_type", "org_id", "country", "config")
                      VALUES('101', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c95d', 'KN', 100, 'RW', '{ "encoding": "smart", "use_national": false }');

/* Contact with id 102 */
DELETE FROM contacts_contact;
INSERT INTO contacts_contact("id", "is_active", "created_on", "modified_on", "uuid", "is_blocked", "is_test", "is_stopped", "language", "created_by_id", "modified_by_id", "org_id")
                      VALUES(102, True, now(), now(), 'a984069d-0008-4d8c-a772-b14a8a6acccc', False, False, False, 'eng', 1, 1, 100);

/** ContactURN with id 103 */
DELETE FROM contacts_contacturn;
INSERT INTO contacts_contacturn("id", "urn", "path", "scheme", "priority", "channel_id", "contact_id", "org_id")
                         VALUES(103, 'tel:+12067799192', '+12067799192', 'tel', 50, 101, 102, 100);

/** Msg with id 104 */
DELETE from msgs_msg;
INSERT INTO msgs_msg("id", "text", "priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility",
                        "has_template_error", "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(104, 'test message', 500, now(), now(), now(), now(), 'O', 'W', 'V',
                     False, 1, 0, now(), 'ext1', 101, 102, 103, 100);