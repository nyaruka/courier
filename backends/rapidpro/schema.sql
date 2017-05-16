CREATE TABLE orgs_org (
    id serial primary key,
    name character varying(255) NOT NULL,
    language character varying(64)
);

CREATE TABLE channels_channel (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL,
    channel_type character varying(3) NOT NULL,
    name character varying(64),
    address character varying(64),
    country character varying(2),
    config text,
    org_id integer references orgs_org(id) on delete cascade
);

CREATE TABLE contacts_contact (
    id serial primary key,
    is_active boolean NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone NOT NULL,
    uuid character varying(36) NOT NULL,
    name character varying(128),
    is_blocked boolean NOT NULL,
    is_test boolean NOT NULL,
    is_stopped boolean NOT NULL,
    language character varying(3),
    created_by_id integer NOT NULL,
    modified_by_id integer NOT NULL,
    org_id integer references orgs_org(id) on delete cascade
);

CREATE TABLE contacts_contacturn (
    id serial primary key,
    urn character varying(255) NOT NULL,
    path character varying(255) NOT NULL,
    scheme character varying(128) NOT NULL,
    priority integer NOT NULL,
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer references contacts_contact(id) on delete cascade,
    org_id integer references orgs_org(id) on delete cascade,
    auth text
);

CREATE TABLE msgs_msg (
    id serial primary key,
    text text NOT NULL,
    priority integer NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone,
    sent_on timestamp with time zone,
    queued_on timestamp with time zone,
    direction character varying(1) NOT NULL,
    status character varying(1) NOT NULL,
    visibility character varying(1) NOT NULL,
    has_template_error boolean NOT NULL,
    msg_type character varying(1),
    msg_count integer NOT NULL,
    error_count integer NOT NULL,
    next_attempt timestamp with time zone NOT NULL,
    external_id character varying(255),
    attachments character varying(255),
    channel_id integer references channels_channel(id) on delete cascade,
    contact_id integer NOT NULL references contacts_contact(id) on delete cascade,
    contact_urn_id integer references contacts_contacturn(id) on delete cascade,
    org_id integer references orgs_org(id) on delete cascade,
    topup_id integer
);