-- +goose Up
-- Derived client debt cache stored as integer cents. Source of truth stays
-- sales + sale_payments.

ALTER TABLE clients
    ADD COLUMN debt_amount INTEGER NOT NULL DEFAULT 0
    CHECK (debt_amount >= 0 AND debt_amount <= 999999999999);

UPDATE clients
SET debt_amount = COALESCE((
    SELECT SUM(CASE
                 WHEN s.total_amount - s.amount_paid > 0
                 THEN s.total_amount - s.amount_paid
                 ELSE 0
               END)
    FROM sales s
    WHERE s.client_id = clients.id
), 0);
