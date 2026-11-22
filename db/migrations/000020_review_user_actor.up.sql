ALTER TABLE delivery_review_history ALTER COLUMN api_key_id DROP NOT NULL;
ALTER TABLE delivery_review_history ADD COLUMN actor_user_id uuid REFERENCES users(id);
ALTER TABLE delivery_review_history ADD CONSTRAINT delivery_review_history_actor_check CHECK ((api_key_id IS NULL) <> (actor_user_id IS NULL));
