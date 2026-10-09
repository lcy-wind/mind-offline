CREATE TABLE IF NOT EXISTS guests (
 id text PRIMARY KEY, token_hash text UNIQUE NOT NULL, balance integer NOT NULL DEFAULT 300 CHECK(balance>=0),
 last_claim date, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS accounts (
 id text PRIMARY KEY REFERENCES guests(id), username text UNIQUE NOT NULL,
 password_hash text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS disabled boolean NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS admin_note text NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_login_at timestamptz;
CREATE TABLE IF NOT EXISTS customer_sessions (
 token_hash text PRIMARY KEY, account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS customer_sessions_account ON customer_sessions(account_id);
CREATE TABLE IF NOT EXISTS music_bindings (
 account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 music_uid text UNIQUE NOT NULL, nickname text NOT NULL, avatar text NOT NULL DEFAULT '',
 cookie_cipher bytea, bound_at timestamptz NOT NULL DEFAULT now(), expired boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS music_link_attempts (
 id text PRIMARY KEY, account_id text UNIQUE NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 session_hash text NOT NULL REFERENCES customer_sessions(token_hash) ON DELETE CASCADE,
 secret bytea NOT NULL, expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS dishes (
 id serial PRIMARY KEY, name text NOT NULL, description text NOT NULL, category text NOT NULL,
 emoji text NOT NULL, price integer NOT NULL CHECK(price BETWEEN 1 AND 200), available boolean NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS orders (
 id text PRIMARY KEY, number bigserial UNIQUE, guest_id text NOT NULL REFERENCES guests(id), request_key text NOT NULL,
 total integer NOT NULL CHECK(total>0), status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','cooking','ready','completed','cancelled')),
 mood text NOT NULL, note text NOT NULL DEFAULT '', quote text NOT NULL, items jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(guest_id,request_key)
);
CREATE INDEX IF NOT EXISTS orders_guest_created ON orders(guest_id,created_at DESC);
CREATE TABLE IF NOT EXISTS admin_sessions (token_hash text PRIMARY KEY, expires_at timestamptz NOT NULL);
INSERT INTO dishes(id,name,description,category,emoji,price) VALUES
 (1,'周一去世拌面','把周一拌开，假装今天是周五。','续命主食','🍜',28),
 (2,'老板画的大饼','0 卡路里，100% 想象力。建议搭配现实食用。','续命主食','🫓',18),
 (3,'躺平蛋包饭','柔软地包住那个不想努力的你。','续命主食','🍳',32),
 (4,'内耗冰美式','一口清醒，两口怀疑人生。','精神饮品','☕',16),
 (5,'已读不回气泡水','消息先放着，气泡不会等你。','精神饮品','🧋',18),
 (6,'反卷柠檬茶','今天的酸，只来自柠檬。','精神饮品','🍋',22),
 (7,'摸鱼薯条','每一根都是带薪快乐。','摸鱼小食','🍟',15),
 (8,'情绪稳定布丁','晃一晃也没关系，你已经很棒了。','摸鱼小食','🍮',20),
 (9,'下班自由套餐','拌面 + 气泡水 + 不回工作消息的勇气。','离职套餐','🍱',48)
 ON CONFLICT(id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('dishes','id'),GREATEST((SELECT COALESCE(max(id),1) FROM dishes),1));
