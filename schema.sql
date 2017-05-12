CREATE TABLE orgs_org (
    "id" serial primary key,
    "name" varchar(255) NOT NULL,
    "language" character varying(64)
);

CREATE TABLE channels_channel (
    "id" serial primary key,
    "is_active" boolean NOT NULL,
    "created_on" timestamp with time zone NOT NULL,
    "modified_on" timestamp with time zone NOT NULL,
    "uuid" character varying(36) NOT NULL,
    "channel_type" character varying(3) NOT NULL,
    "name" character varying(64),
    "address" character varying(64),
    "country" character varying(2),
    "config" text,
    "org_id" integer REFERENCES orgs_org (id) on delete cascade
);

CREATE TABLE contacts_contact (
    "id" serial primary key,
    "is_active" boolean NOT NULL,
    "created_on" timestamp with time zone NOT NULL,
    "modified_on" timestamp with time zone NOT NULL,
    "uuid" character varying(36) NOT NULL,
    "name" character varying(128),
    "is_blocked" boolean NOT NULL,
    "is_test" boolean NOT NULL,
    "is_stopped" boolean NOT NULL,
    "language" character varying(3),
    "created_by_id" integer NOT NULL,
    "modified_by_id" integer NOT NULL,
    "org_id" integer REFERENCES orgs_org (id)
);

CREATE TABLE contacts_contacturn (
    "id" serial primary key,
    "urn" character varying(255) NOT NULL,
    "path" character varying(255) NOT NULL,
    "scheme" character varying(128) NOT NULL,
    "priority" integer NOT NULL,
    "channel_id" integer REFERENCES channels_channel (id),
    "contact_id" integer REFERENCES contacts_contact (id),
    "org_id" integer REFERENCES orgs_org (id),
    "auth" text
);

CREATE TABLE msgs_msg (
    "id" serial primary key,
    "text" text NOT NULL,
    "priority" integer NOT NULL,
    "created_on" timestamp with time zone NOT NULL,
    "modified_on" timestamp with time zone,
    "sent_on" timestamp with time zone,
    "queued_on" timestamp with time zone,
    "direction" character varying(1) NOT NULL,
    "status" character varying(1) NOT NULL,
    "visibility" character varying(1) NOT NULL,
    "has_template_error" boolean NOT NULL,
    "msg_type" character varying(1),
    "msg_count" integer NOT NULL,
    "error_count" integer NOT NULL,
    "next_attempt" timestamp with time zone NOT NULL,
    "external_id" character varying(255),
    "attachments" character varying(255),
    "channel_id" integer REFERENCES channels_channel (id),
    "contact_id" integer NOT NULL REFERENCES contacts_contact (id),
    "contact_urn_id" integer REFERENCES contacts_contacturn (id),
    "org_id" integer REFERENCES orgs_org (id),
    "topup_id" integer
);
