DROP TABLE IF EXISTS users_user CASCADE;
CREATE TABLE users_user (
    id serial primary key,
    email character varying(254) NOT NULL UNIQUE,
    first_name character varying(150) NOT NULL
);

DROP TABLE IF EXISTS orgs_org CASCADE;
CREATE TABLE orgs_org (
    id serial primary key,
    name character varying(128) NOT NULL,
    language character varying(64),
    config jsonb NOT NULL,
    is_anon boolean NOT NULL
);

DROP TABLE IF EXISTS channels_channel CASCADE;
CREATE TABLE channels_channel (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid uuid NOT NULL UNIQUE,
    channel_type character varying(3) NOT NULL,
    name character varying(64) NOT NULL,
    address character varying(255),
    country character varying(2),
    config jsonb NOT NULL,
    schemes character varying(16)[] NOT NULL,
    role character varying(4) NOT NULL,
    org_id integer references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS contacts_contact CASCADE;
CREATE TABLE contacts_contact (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL UNIQUE,
    name character varying(128),
    language character varying(3),
    status character varying(1) NOT NULL,
    ticket_count integer NOT NULL,
    created_by_id integer references users_user(id),
    modified_by_id integer references users_user(id),
    org_id integer NOT NULL references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS contacts_contacturn CASCADE;
CREATE TABLE contacts_contacturn (
    id serial primary key,
    identity character varying(255) NOT NULL,
    scheme character varying(128) NOT NULL,
    path character varying(255) NOT NULL,
    display character varying(255) NULL,
    priority integer NOT NULL,
    auth_tokens jsonb,
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer references contacts_contact(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    UNIQUE (org_id, identity)
);

DROP TABLE IF EXISTS contacts_contactfire CASCADE;
CREATE TABLE contacts_contactfire (
    id bigserial primary key,
    fire_type character varying(1) NOT NULL,
    scope character varying(64) NOT NULL,
    fire_on timestamp with time zone NOT NULL,
    session_uuid uuid,
    sprint_uuid uuid,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    UNIQUE (contact_id, fire_type, scope)
);

DROP TABLE IF EXISTS msgs_msg CASCADE;
CREATE TABLE msgs_msg (
    id bigserial PRIMARY KEY,
    uuid uuid NOT NULL UNIQUE,
    text text NOT NULL,
    attachments character varying(2048)[] NULL,
    quickreplies jsonb NULL,
    locale character varying(6) NULL,
    templating jsonb NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    sent_on timestamp with time zone,
    msg_type character varying(1) NOT NULL,
    direction character varying(1) NOT NULL,
    status character varying(1) NOT NULL,
    visibility character varying(1) NOT NULL,
    is_android boolean NOT NULL,
    msg_count integer NOT NULL,
    high_priority boolean NULL,
    error_count integer NOT NULL,
    next_attempt timestamp with time zone,
    failed_reason character varying(1),
    log_uuids uuid[],
    --broadcast_id integer REFERENCES msgs_broadcast(id) ON DELETE CASCADE,
    channel_id integer REFERENCES channels_channel(id) ON DELETE CASCADE,
    contact_id integer NOT NULL REFERENCES contacts_contact(id) ON DELETE CASCADE,
    contact_urn_id integer REFERENCES contacts_contacturn(id) ON DELETE CASCADE,
    created_by_id integer REFERENCES users_user(id),
    --flow_id integer REFERENCES flows_flow(id) ON DELETE CASCADE,
    org_id integer NOT NULL REFERENCES orgs_org(id) ON DELETE CASCADE,
    --ticket_uuid uuid,
    external_identifier character varying(255)
);

DROP TABLE IF EXISTS channels_channelevent CASCADE;
CREATE TABLE channels_channelevent (
    id bigserial primary key,
    uuid uuid NOT NULL UNIQUE,
    event_type character varying(16) NOT NULL,
    status character varying(1),
    extra text,
    occurred_on timestamp with time zone NOT NULL,
    created_on timestamp with time zone NOT NULL,
    log_uuids uuid[],
    channel_id integer NOT NULL references channels_channel(id) on delete cascade,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    contact_urn_id integer references contacts_contacturn(id) on delete cascade,
    org_id integer NOT NULL references orgs_org(id) on delete cascade
);

DROP TABLE IF EXISTS msgs_media CASCADE;
CREATE TABLE msgs_media (
    id bigserial primary key,
    uuid uuid NOT NULL UNIQUE,
    url character varying(2048) NOT NULL,
    content_type character varying(255) NOT NULL,
    path character varying(2048) NOT NULL,
    size integer NOT NULL,
    duration integer NOT NULL,
    width integer NOT NULL,
    height integer NOT NULL,
    org_id integer NOT NULL references orgs_org(id) on delete cascade,
    original_id bigint references msgs_media(id) on delete cascade
);

GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier_test;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier_test;
