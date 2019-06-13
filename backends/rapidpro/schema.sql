DROP TABLE IF EXISTS orgs_org CASCADE;
CREATE TABLE orgs_org (
    id serial primary key,
    name character varying(255) NOT NULL,
    language character varying(64),
    is_anon boolean NOT NULL,
    config text NULL
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
    config text,
    org_id integer references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS contacts_contact CASCADE;
CREATE TABLE contacts_contact (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL,
    name character varying(128),
    is_blocked boolean NOT NULL,
    is_stopped boolean NOT NULL,
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
    auth text,
    UNIQUE (org_id, identity)
);

DROP TABLE IF EXISTS msgs_msg CASCADE;
CREATE TABLE msgs_msg (
    id serial primary key,
    uuid character varying(36) NULL,
    text text NOT NULL,
    high_priority boolean NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone,
    sent_on timestamp with time zone,
    queued_on timestamp with time zone,
    direction character varying(1) NOT NULL,
    status character varying(1) NOT NULL,
    visibility character varying(1) NOT NULL,
    msg_type character varying(1),
    msg_count integer NOT NULL,
    error_count integer NOT NULL,
    next_attempt timestamp with time zone NOT NULL,
    external_id character varying(255),
    attachments character varying(255)[],
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    contact_urn_id integer NOT NULL references contacts_contacturn(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    metadata text,
    topup_id integer
);

DROP TABLE IF EXISTS channels_channellog CASCADE;
CREATE TABLE channels_channellog (
    id serial primary key,
    description character varying(255) NOT NULL,
    is_error boolean NOT NULL,
    url text,
    method character varying(16),
    request text,
    response text,
    response_status integer,
    created_on timestamp with time zone NOT NULL,
    request_time integer,
    channel_id integer NOT NULL references channels_channel(id) on delete cascade,
    msg_id integer references msgs_msg(id) on delete cascade,
    session_id integer NULL
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
    org_id integer NOT NULL references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS flows_flowsession CASCADE;
CREATE TABLE flows_flowsession (
    id serial primary key,
    status character varying(1) NOT NULL,
    timeout_on timestamp with time zone NULL,
    wait_started_on timestamp with time zone
);

GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier;