/* Org with id 1 */
DELETE FROM orgs_org;
INSERT INTO orgs_org("id", "name", "language", "is_anon", "config")
              VALUES(1, 'Test Org', 'eng', FALSE, '{ "CHATBASE_API_KEY": "cak" }');

/* Channel with id 10, 11, 12 */
DELETE FROM channels_channel;
INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "config")
                      VALUES('10', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c95d', 'KN', '2500', 1, 'RW', '{ "encoding": "smart", "use_national": true, "max_length_int": 320, "max_length_str": "320" }');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "config")
                      VALUES('11', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c96a', 'TW', '4500', 1, 'US', NULL);

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "config")
                      VALUES('12', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c97a', 'DM', '4500', 1, 'US', NULL);                      

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "config")
                      VALUES('13', '{"telegram"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c98a', 'TG', 'courierbot', 1, NULL, NULL);                                            

/* Contacts with ids 100, 101 */
DELETE FROM contacts_contact;
INSERT INTO contacts_contact("id", "is_active", "created_on", "modified_on", "uuid", "is_blocked", "is_stopped", "language", "created_by_id", "modified_by_id", "org_id")
                      VALUES(100, True, now(), now(), 'a984069d-0008-4d8c-a772-b14a8a6acccc', False, False, 'eng', 1, 1, 1);

/** ContactURN with id 1000 */
DELETE FROM contacts_contacturn;
INSERT INTO contacts_contacturn("id", "identity", "path", "scheme", "priority", "channel_id", "contact_id", "org_id")
                         VALUES(1000, 'tel:+12067799192', '+12067799192', 'tel', 50, 10, 100, 1);

/** Msg with id 10,000 */
DELETE from msgs_msg;
INSERT INTO msgs_msg("id", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility",
                        "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(10000, 'test message', True, now(), now(), now(), now(), 'O', 'W', 'V',
                     1, 0, now(), 'ext1', 10, 100, 1000, 1);

INSERT INTO msgs_msg("id", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility",
                        "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(10001, 'test message without external', True, now(), now(), now(), now(), 'O', 'W', 'V',
                     1, 0, now(), 'ext1', 10, 100, 1000, 1);

/** Simple session */
DELETE from flows_flowsession;
INSERT INTO flows_flowsession("id", "status", "wait_started_on")
                       VALUES(1, 'W', '2018-12-04 11:52:20.958955-08'),
                             (2, 'C', '2018-12-04 11:52:20.958955-08');
