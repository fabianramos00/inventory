-- +goose Up
-- Sales and sale items.

--
-- Name: sale_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sale_items (
    id integer NOT NULL,
    sale_id integer,
    product_id integer,
    quantity integer NOT NULL,
    unit_price numeric(12,2) NOT NULL,
    subtotal numeric(12,2) NOT NULL,
    delivered_quantity integer NOT NULL,
    CONSTRAINT ck_sale_items_delivered_valid CHECK (((delivered_quantity >= 0) AND (delivered_quantity <= quantity))),
    CONSTRAINT ck_sale_items_quantity_positive CHECK ((quantity > 0))
);

--
-- Name: sale_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sale_items_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: sale_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sale_items_id_seq OWNED BY public.sale_items.id;

--
-- Name: sales; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sales (
    id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone,
    total_amount numeric(12,2) NOT NULL,
    amount_paid numeric(12,2) NOT NULL,
    payment_status public.payment_status DEFAULT 'PENDING'::public.payment_status NOT NULL,
    payment_method public.payment_method,
    created_by_id integer,
    delivery_status public.delivery_status DEFAULT 'PENDING'::public.delivery_status NOT NULL,
    client_id integer,
    CONSTRAINT ck_sales_total_amount_nonnegative CHECK (((total_amount)::double precision >= (0)::double precision))
);

--
-- Name: sales_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sales_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: sales_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sales_id_seq OWNED BY public.sales.id;

--
-- Name: sale_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sale_items ALTER COLUMN id SET DEFAULT nextval('public.sale_items_id_seq'::regclass);

--
-- Name: sales id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sales ALTER COLUMN id SET DEFAULT nextval('public.sales_id_seq'::regclass);

--
-- Name: sale_items sale_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sale_items
    ADD CONSTRAINT sale_items_pkey PRIMARY KEY (id);

--
-- Name: sales sales_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sales
    ADD CONSTRAINT sales_pkey PRIMARY KEY (id);

--
-- Name: ix_sale_items_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sale_items_id ON public.sale_items USING btree (id);

--
-- Name: ix_sale_items_product_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sale_items_product_id ON public.sale_items USING btree (product_id);

--
-- Name: ix_sale_items_sale_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sale_items_sale_id ON public.sale_items USING btree (sale_id);

--
-- Name: ix_sales_client_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sales_client_id ON public.sales USING btree (client_id);

--
-- Name: ix_sales_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sales_created_at ON public.sales USING btree (created_at);

--
-- Name: ix_sales_delivery_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sales_delivery_status ON public.sales USING btree (delivery_status);

--
-- Name: ix_sales_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sales_id ON public.sales USING btree (id);

--
-- Name: ix_sales_payment_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_sales_payment_status ON public.sales USING btree (payment_status);

--
-- Name: sales fk_sales_client_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sales
    ADD CONSTRAINT fk_sales_client_id FOREIGN KEY (client_id) REFERENCES public.clients(id);

--
-- Name: sale_items sale_items_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sale_items
    ADD CONSTRAINT sale_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.products(id);

--
-- Name: sale_items sale_items_sale_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sale_items
    ADD CONSTRAINT sale_items_sale_id_fkey FOREIGN KEY (sale_id) REFERENCES public.sales(id);

--
-- Name: sales sales_created_by_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sales
    ADD CONSTRAINT sales_created_by_id_fkey FOREIGN KEY (created_by_id) REFERENCES public.users(id);
