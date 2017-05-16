create table channels_channel (
	org_id serial not null,
	id serial not null constraint channels_channel_pkey primary key,
	uuid varchar(36) not null constraint channels_channel_uuid_key unique,
	channel_type varchar(3) not null,
	address varchar(64),
	country varchar(2),
	config text,
	is_active boolean not null
);

create table msgs_msg (
	id serial not null constraint msgs_msg_pkey primary key, 
	external_id varchar(255),
	channel_id integer constraint msgs_msg_channel_id_fk_channels_channel_id references channels_channel deferrable initially deferred,
	uuid uuid
);
