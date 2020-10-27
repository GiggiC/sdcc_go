-- Adminer 4.7.7 PostgreSQL dump

CREATE SEQUENCE messages_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 2147483647 START 363 CACHE 1;

CREATE TABLE IF NOT EXISTS "public"."topics" (
    "name" text NOT NULL,
    CONSTRAINT "topics_pk" PRIMARY KEY ("name")
) WITH (oids = false);

CREATE TABLE IF NOT EXISTS "public"."users" (
    "email" text NOT NULL,
    "password" text,
    CONSTRAINT "users_pk" PRIMARY KEY ("email")
) WITH (oids = false);

CREATE TABLE IF NOT EXISTS "public"."messages" (
    "payload" text,
    "topic" text,
    "id" integer DEFAULT nextval('messages_id_seq') NOT NULL,
    "radius" integer,
    "latitude" text,
    "longitude" text,
    "lifetime" timestamp,
    "title" text,
    CONSTRAINT "messages_pk" PRIMARY KEY ("id"),
    CONSTRAINT "messages_topics_name_fk" FOREIGN KEY (topic) REFERENCES topics(name) ON UPDATE CASCADE ON DELETE CASCADE NOT DEFERRABLE
) WITH (oids = false);

CREATE TABLE IF NOT EXISTS "public"."subscriptions" (
    "subscriber" text NOT NULL,
    "topic" text NOT NULL,
    CONSTRAINT "subscriptions_pk" PRIMARY KEY ("subscriber", "topic"),
    CONSTRAINT "subscriptions_topics_name_fk" FOREIGN KEY (topic) REFERENCES topics(name) ON UPDATE CASCADE ON DELETE CASCADE NOT DEFERRABLE,
    CONSTRAINT "subscriptions_users_email_fk" FOREIGN KEY (subscriber) REFERENCES users(email) ON UPDATE CASCADE ON DELETE CASCADE NOT DEFERRABLE
) WITH (oids = false);

INSERT INTO "topics" ("name") VALUES
('Elettronica'),
('Informatica'),
('Arredamento'),
('Abbigliamento'),
('Tutto per i bambini'),
('Giardino e Fai da te'),
('Elettrodomestici'),
('Animali'),
('Sport'),
('Libri e Riviste'),
('Strumenti musicali'),
('Appartamenti'),
('Offerte di lavoro'),
('Auto'),
('Moto'),
('Casa'),
('Videogames')
ON CONFLICT DO NOTHING;