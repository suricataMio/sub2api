-- 为告警规则添加 notify_telegram 列（支持 Telegram 机器人通知）
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS notify_telegram BOOLEAN NOT NULL DEFAULT FALSE;

-- 为告警事件添加 telegram_sent 列（记录是否已发送 Telegram 通知）
ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS telegram_sent BOOLEAN NOT NULL DEFAULT FALSE;
