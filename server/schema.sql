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

-- Add the expanded menu once; generated IDs preserve dishes created by the shop owner.
CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
WITH applied AS (
 INSERT INTO schema_migrations(version) VALUES('20261010_expanded_menu_v1') ON CONFLICT DO NOTHING RETURNING version
)
INSERT INTO dishes(name,description,category,emoji,price)
SELECT name,description,category,emoji,price FROM (VALUES
 ('已读乱回炒饭','米饭粒粒分明，工作消息句句不回。','续命主食','🍚',26),
 ('带薪摸鱼酸菜鱼','鱼已经摸好了，酸菜负责替你阴阳怪气。','续命主食','🐟',38),
 ('拒绝加班咖喱饭','咖喱可以加，今天的班不能加。','续命主食','🍛',32),
 ('需求冻结牛肉面','牛肉加满，需求请下个版本再提。','续命主食','🥩',35),
 ('周五快乐汉堡','两片面包夹住五天的委屈，快乐大口吃。','续命主食','🍔',29),
 ('撤回消息奶茶','奶茶三分糖，刚才那句话当我没说。','精神饮品','🧋',19),
 ('开会静音拿铁','麦克风已关闭，咖啡因已上线。','精神饮品','☕',24),
 ('周报兑水柠檬水','工作内容适量展开，柠檬水不掺假。','精神饮品','🍋',12),
 ('情绪缓冲热可可','先缓冲一下，世界不差你这五分钟。','精神饮品','🍫',23),
 ('下班倒计时橙汁','维生素补上，今天的进度条快走完。','精神饮品','🍊',18),
 ('老板别叭叭鸡米花','嘴巴用来吃鸡米花，就没空听画饼。','摸鱼小食','🍗',22),
 ('周报压缩小蛋糕','把一周的辛苦，压缩成一口甜。','摸鱼小食','🍰',21),
 ('会议逃生甜甜圈','甜甜圈中间的洞，是通往下班的出口。','摸鱼小食','🍩',16),
 ('已完成烤肠','不用再改了，这根烤肠已经最终最终版。','摸鱼小食','🌭',14),
 ('周末预支冰淇淋','先尝一口周末，融化的只有烦恼。','摸鱼小食','🍨',18),
 ('周一重启套餐','咖喱饭 + 冰美式。重启大脑，不重启工作群。','离职套餐','🔋',45),
 ('带薪摸鱼双人餐','酸菜鱼 + 炒饭 + 两杯柠檬水，快乐找人平摊。','离职套餐','🐠',76),
 ('拒绝内耗快乐餐','汉堡 + 薯条 + 橙汁。今天只消耗卡路里。','离职套餐','🍟',52),
 ('工位隐身下午茶','拿铁 + 小蛋糕，暂时把在线状态设为离线。','离职套餐','🫖',39),
 ('精神离职毕业餐','牛肉面 + 热可可 + 甜甜圈，祝你准时下班。','离职套餐','🎓',66)
) AS additions(name,description,category,emoji,price)
WHERE EXISTS(SELECT 1 FROM applied);
