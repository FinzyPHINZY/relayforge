DELETE FROM delivery_review_history WHERE api_key_id IS NULL;
ALTER TABLE delivery_review_history DROP CONSTRAINT delivery_review_history_actor_check;
ALTER TABLE delivery_review_history DROP COLUMN actor_user_id;
ALTER TABLE delivery_review_history ALTER COLUMN api_key_id SET NOT NULL;
