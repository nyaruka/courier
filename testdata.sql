
DELETE FROM orgs_org where id = 99;
INSERT INTO orgs_org("id", "name", "language") 
    VALUES(99, 'Test Org', 'eng');

DELETE FROM channels_channel WHERE uuid = 'a984069d-0008-4d8c-a772-b14a8a6abbdb';
INSERT INTO channels_channel("is_active", "created_on", "modified_on", "uuid", "channel_type", "org_id") 
    VALUES('Y', NOW(), NOW(), 'a984069d-0008-4d8c-a772-b14a8a6abbdb', 'KN', 99);