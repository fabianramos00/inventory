-- +goose Up
-- Purchase orders, order items, and supplier catalogs.

--
-- Name: order_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_items (
    id integer NOT NULL,
    order_id integer NOT NULL,
    product_id integer NOT NULL,
    quantity integer NOT NULL,
    unit_cost numeric(12,2) NOT NULL,
    subtotal numeric(12,2) NOT NULL,
    supplier_sku character varying(100),
    CONSTRAINT ck_order_items_quantity_positive CHECK ((quantity > 0))
);

--
-- Name: order_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_items_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: order_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_items_id_seq OWNED BY public.order_items.id;

--
-- Name: purchase_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_orders (
    id integer NOT NULL,
    provider_id integer,
    status public.order_status DEFAULT 'PENDING'::public.order_status NOT NULL,
    order_date timestamp with time zone DEFAULT now(),
    total_amount numeric(12,2) NOT NULL,
    created_by_id integer,
    payment_status public.purchase_order_payment_status DEFAULT 'PENDING'::public.purchase_order_payment_status NOT NULL,
    CONSTRAINT ck_purchase_orders_total_amount_nonnegative CHECK (((total_amount)::double precision >= (0)::double precision))
);

--
-- Name: purchase_orders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.purchase_orders_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: purchase_orders_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.purchase_orders_id_seq OWNED BY public.purchase_orders.id;

--
-- Name: supplier_catalogs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supplier_catalogs (
    id integer NOT NULL,
    product_id integer,
    provider_id integer,
    supplier_sku character varying(100),
    last_quoted_cost numeric(12,2) NOT NULL,
    lead_time integer
);

--
-- Name: supplier_catalogs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.supplier_catalogs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: supplier_catalogs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.supplier_catalogs_id_seq OWNED BY public.supplier_catalogs.id;

--
-- Name: order_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items ALTER COLUMN id SET DEFAULT nextval('public.order_items_id_seq'::regclass);

--
-- Name: purchase_orders id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_orders ALTER COLUMN id SET DEFAULT nextval('public.purchase_orders_id_seq'::regclass);

--
-- Name: supplier_catalogs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_catalogs ALTER COLUMN id SET DEFAULT nextval('public.supplier_catalogs_id_seq'::regclass);

--
-- Name: order_items order_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_pkey PRIMARY KEY (id);

--
-- Name: purchase_orders purchase_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_orders
    ADD CONSTRAINT purchase_orders_pkey PRIMARY KEY (id);

--
-- Name: supplier_catalogs supplier_catalogs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_catalogs
    ADD CONSTRAINT supplier_catalogs_pkey PRIMARY KEY (id);

--
-- Name: ix_order_items_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_order_items_id ON public.order_items USING btree (id);

--
-- Name: ix_order_items_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_order_items_order_id ON public.order_items USING btree (order_id);

--
-- Name: ix_order_items_product_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_order_items_product_id ON public.order_items USING btree (product_id);

--
-- Name: ix_purchase_orders_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_purchase_orders_id ON public.purchase_orders USING btree (id);

--
-- Name: ix_purchase_orders_order_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_purchase_orders_order_date ON public.purchase_orders USING btree (order_date);

--
-- Name: ix_purchase_orders_provider_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_purchase_orders_provider_id ON public.purchase_orders USING btree (provider_id);

--
-- Name: ix_purchase_orders_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_purchase_orders_status ON public.purchase_orders USING btree (status);

--
-- Name: ix_supplier_catalogs_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_supplier_catalogs_id ON public.supplier_catalogs USING btree (id);

--
-- Name: order_items order_items_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.purchase_orders(id);

--
-- Name: order_items order_items_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.products(id);

--
-- Name: purchase_orders purchase_orders_created_by_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_orders
    ADD CONSTRAINT purchase_orders_created_by_id_fkey FOREIGN KEY (created_by_id) REFERENCES public.users(id);

--
-- Name: purchase_orders purchase_orders_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_orders
    ADD CONSTRAINT purchase_orders_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.providers(id);

--
-- Name: supplier_catalogs supplier_catalogs_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_catalogs
    ADD CONSTRAINT supplier_catalogs_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.products(id);

--
-- Name: supplier_catalogs supplier_catalogs_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_catalogs
    ADD CONSTRAINT supplier_catalogs_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.providers(id);
