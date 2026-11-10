-- +goose Up
-- Derived client debt cache. Source of truth stays sales + sale_payments.

ALTER TABLE public.clients
    ADD COLUMN debt_amount numeric(12,2) NOT NULL DEFAULT 0;

ALTER TABLE public.clients
    ADD CONSTRAINT ck_clients_debt_amount_nonnegative
    CHECK (debt_amount >= 0);

UPDATE public.clients c
SET debt_amount = d.debt
FROM (
    SELECT client_id,
           SUM(GREATEST(total_amount - amount_paid, 0)) AS debt
    FROM public.sales
    WHERE client_id IS NOT NULL
    GROUP BY client_id
) d
WHERE d.client_id = c.id;
