DROP TABLE IF EXISTS orgs_org CASCADE;
CREATE TABLE orgs_org (
    id serial primary key,
    name character varying(255) NOT NULL,
    language character varying(64),
    is_anon boolean NOT NULL,
    config jsonb NOT NULL
);

DROP TABLE IF EXISTS channels_channel CASCADE;
CREATE TABLE channels_channel (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL,
    channel_type character varying(3) NOT NULL,
    name character varying(64),
    schemes character varying(16)[] NOT NULL,
    address character varying(64),
    country character varying(2),
    config jsonb NOT NULL,
    role character varying(4) NOT NULL,
    log_policy character varying(1) NOT NULL,
    org_id integer references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS contacts_contact CASCADE;
CREATE TABLE contacts_contact (
    id serial primary key,
    is_active boolean NOT NULL,
    status character varying(1) NOT NULL,
    ticket_count integer NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL,
    name character varying(128),
    language character varying(3),
    created_by_id integer NOT NULL,
    modified_by_id integer NOT NULL,
    org_id integer references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS contacts_contacturn CASCADE;
CREATE TABLE contacts_contacturn (
    id serial primary key,
    identity character varying(255) NOT NULL,
    path character varying(255) NOT NULL,
    scheme character varying(128) NOT NULL,
    display character varying(128) NULL,
    priority integer NOT NULL,
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer references contacts_contact(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    auth_tokens jsonb,
    UNIQUE (org_id, identity)
);

DROP TABLE IF EXISTS msgs_optin CASCADE;
CREATE TABLE msgs_optin (
    id serial primary key,
    uuid uuid NOT NULL,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    name character varying(64)
);

DROP TABLE IF EXISTS msgs_msg CASCADE;
CREATE TABLE msgs_msg (
    id bigserial primary key,
    uuid uuid NULL,
    text text NOT NULL,
    attachments character varying(255)[] NULL,
    quick_replies character varying(64)[] NULL,
    locale character varying(6) NULL,
    high_priority boolean NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone,
    sent_on timestamp with time zone,
    queued_on timestamp with time zone,
    direction character varying(1) NOT NULL,
    status character varying(1) NOT NULL,
    visibility character varying(1) NOT NULL,
    msg_type character varying(1) NOT NULL,
    msg_count integer NOT NULL,
    error_count integer NOT NULL,
    next_attempt timestamp with time zone NOT NULL,
    failed_reason character varying(1),
    external_id character varying(255),
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    contact_urn_id integer NOT NULL references contacts_contacturn(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    metadata text,
    optin_id integer references msgs_optin(id) on delete cascade,
    delete_from_counts boolean,
    log_uuids uuid[]
);

DROP TABLE IF EXISTS channels_channellog CASCADE;
CREATE TABLE channels_channellog (
    id serial primary key,
    uuid uuid NOT NULL,
    channel_id integer NOT NULL references channels_channel(id) on delete cascade,
    log_type character varying(16),
    http_logs jsonb,
    errors jsonb,
    is_error boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    elapsed_ms integer
);

DROP TABLE IF EXISTS channels_channelevent CASCADE;
CREATE TABLE channels_channelevent (
    id serial primary key,
    event_type character varying(16) NOT NULL,
    extra text,
    occurred_on timestamp with time zone NOT NULL,
    created_on timestamp with time zone NOT NULL,
    channel_id integer NOT NULL references channels_channel(id) on delete cascade,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    contact_urn_id integer NOT NULL references contacts_contacturn(id) on delete cascade,
    optin_id integer references msgs_optin(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    log_uuids uuid[]
);

DROP TABLE IF EXISTS flows_flowsession CASCADE;
CREATE TABLE flows_flowsession (
    id serial primary key,
    status character varying(1) NOT NULL,
    timeout_on timestamp with time zone NULL,
    wait_started_on timestamp with time zone
);

DROP TABLE IF EXISTS msgs_media CASCADE;
CREATE TABLE IF NOT EXISTS msgs_media (
    id serial primary key,
    uuid uuid NOT NULL,
    org_id integer NOT NULL,
    content_type character varying(255) NOT NULL,
    url character varying(2048) NOT NULL,
    path character varying(2048) NOT NULL,
    size integer NOT NULL,
    duration integer NOT NULL,
    width integer NOT NULL,
    height integer NOT NULL,
    original_id integer
);

GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier_test;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier_test;