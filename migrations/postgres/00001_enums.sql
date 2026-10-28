-- +goose Up
-- Initial schema translated from the legacy Alembic head (PostgreSQL 17).

--
-- Name: delivery_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.delivery_status AS ENUM (
    'PENDING',
    'PARTIAL',
    'DELIVERED'
);

--
-- Name: order_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.order_status AS ENUM (
    'PENDING',
    'SENT',
    'RECEIVED',
    'CANCELLED'
);

--
-- Name: payment_method; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.payment_method AS ENUM (
    'CASH',
    'CREDIT',
    'DEBIT'
);

--
-- Name: payment_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.payment_status AS ENUM (
    'PAID',
    'PENDING',
    'PARTIAL'
);

--
-- Name: purchase_order_payment_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.purchase_order_payment_status AS ENUM (
    'PENDING',
    'PAID'
);
