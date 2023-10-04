/* Org with id 1 */
DELETE FROM orgs_org;
INSERT INTO orgs_org("id", "name", "language", "is_anon", "config")
              VALUES(1, 'Test Org', 'eng', FALSE, '{ "CHATBASE_API_KEY": "cak" }');

/* Channel with id 10, 11, 12 */
DELETE FROM channels_channel;
INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('10', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c95d', 'KN', '2500', 1, 'RW', 'SR', 'A', '{ "encoding": "smart", "use_national": true, "max_length_int": 320, "max_length_str": "320" }');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('11', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c96a', 'TW', '4500', 1, 'US', 'SR', 'A', '{}');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('12', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c97a', 'DM', '4500', 1, 'US', 'SR', 'A', '{}');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('13', '{"telegram"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c98a', 'TG', 'courierbot', 1, NULL, 'SR', 'A', '{}');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('14', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327c99a', 'KN', NULL, 1, 'US', 'SR', 'A', '{}');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('15', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327100a', 'EX', NULL, 1, 'US', 'R', 'A', '{}');

INSERT INTO channels_channel("id", "schemes", "is_active", "created_on", "modified_on", "uuid", "channel_type", "address", "org_id", "country", "role", "log_policy", "config")
                      VALUES('16', '{"tel"}', 'Y', NOW(), NOW(), 'dbc126ed-66bc-4e28-b67b-81dc3327222a', 'EX', NULL, 1, 'US', '', 'A', '{}');

/* Contacts with ids 100, 101 */
DELETE FROM contacts_contact;
INSERT INTO contacts_contact("id", "is_active", "status", "created_on", "modified_on", "uuid", "language", "ticket_count", "created_by_id", "modified_by_id", "org_id")
                      VALUES(100, True, 'A', now(), now(), 'a984069d-0008-4d8c-a772-b14a8a6acccc', 'eng', 0, 1, 1, 1);

/** ContactURN with id 1000 */
DELETE FROM contacts_contacturn;
INSERT INTO contacts_contacturn("id", "identity", "path", "scheme", "priority", "channel_id", "contact_id", "org_id")
                         VALUES(1000, 'tel:+12067799192', '+12067799192', 'tel', 50, 10, 100, 1);

/* Msg optins with ids 1, 2 */
DELETE FROM msgs_optin;
INSERT INTO msgs_optin(id, uuid, org_id, name) VALUES
                      (1, 'fc1cef6e-b5b1-452d-9528-a4b24db28eb0', 1, 'Polls'),
                      (2, '2b1eba23-4a97-46ac-9022-11304412b32f', 1, 'Jokes');

/** Msg with id 10000 */
DELETE FROM msgs_msg;
INSERT INTO msgs_msg("id", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility", "msg_type",
                        "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(10000, 'test message', True, now(), now(), now(), now(), 'O', 'W', 'V', 'T',
                     1, 0, now(), 'ext1', 10, 100, 1000, 1);

INSERT INTO msgs_msg("id", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility", "msg_type",
                        "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(10001, 'test message without external', True, now(), now(), now(), now(), 'O', 'W', 'V', 'T',
                     1, 0, now(), '', 10, 100, 1000, 1);

INSERT INTO msgs_msg("id", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility", "msg_type",
                        "msg_count", "error_count", "next_attempt", "external_id", "channel_id", "contact_id", "contact_urn_id", "org_id")
              VALUES(10002, 'test message incoming', True, now(), now(), now(), now(), 'I', 'P', 'V', 'T',
                     1, 0, now(), 'ext2', 10, 100, 1000, 1);

INSERT INTO msgs_media("id", "uuid", "org_id", "content_type", "url", "path", "size", "duration", "width", "height", "original_id")
                VALUES(100, 'ec6972be-809c-4c8d-be59-ba9dbd74c977', 1, 'image/jpeg', 'http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg', '/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg', 123, 0, 1024, 768, NULL);
INSERT INTO msgs_media("id", "uuid", "org_id", "content_type", "url", "path", "size", "duration", "width", "height", "original_id")
                VALUES(101, '5310f50f-9c8e-4035-9150-be5a1f78f21a', 1, 'audio/mp3', 'http://nyaruka.s3.com/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3', '/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3', 123, 500, 0, 0, NULL);
INSERT INTO msgs_media("id", "uuid", "org_id", "content_type", "url", "path", "size", "duration", "width", "height", "original_id")
                VALUES(102, '514c552c-e585-40e2-938a-fe9450172da8', 1, 'audio/mp4', 'http://nyaruka.s3.com/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a', '/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a', 114, 500, 0, 0, 101);

/** Simple session */
DELETE from flows_flowsession;
INSERT INTO flows_flowsession("id", "status", "wait_started_on")
                       VALUES(1, 'W', '2018-12-04 11:52:20.958955-08'),
                             (2, 'C', '2018-12-04 11:52:20.958955-08');
