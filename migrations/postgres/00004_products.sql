-- +goose Up
-- Products and price history.

--
-- Name: price_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.price_history (
    id integer NOT NULL,
    product_id integer,
    old_price numeric(12,2) NOT NULL,
    new_price numeric(12,2) NOT NULL,
    changed_at timestamp with time zone DEFAULT now()
);

--
-- Name: price_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.price_history_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: price_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.price_history_id_seq OWNED BY public.price_history.id;

--
-- Name: products; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.products (
    id integer NOT NULL,
    sku character varying(100) NOT NULL,
    name character varying(150) NOT NULL,
    size_value character varying(20),
    measurement_unit_id integer,
    price numeric(12,2) NOT NULL,
    cost numeric(12,2) NOT NULL,
    stock_quantity integer,
    low_stock_threshold integer,
    brand_id integer,
    category_id integer,
    material_id integer,
    CONSTRAINT ck_products_cost_positive CHECK (((cost)::double precision > (0)::double precision)),
    CONSTRAINT ck_products_price_positive CHECK (((price)::double precision > (0)::double precision)),
    CONSTRAINT ck_products_stock_nonnegative CHECK ((stock_quantity >= 0))
);

--
-- Name: products_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.products_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: products_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.products_id_seq OWNED BY public.products.id;

--
-- Name: price_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.price_history ALTER COLUMN id SET DEFAULT nextval('public.price_history_id_seq'::regclass);

--
-- Name: products id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products ALTER COLUMN id SET DEFAULT nextval('public.products_id_seq'::regclass);

--
-- Name: price_history price_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.price_history
    ADD CONSTRAINT price_history_pkey PRIMARY KEY (id);

--
-- Name: products products_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_pkey PRIMARY KEY (id);

--
-- Name: ix_price_history_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_price_history_id ON public.price_history USING btree (id);

--
-- Name: ix_products_brand_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_products_brand_id ON public.products USING btree (brand_id);

--
-- Name: ix_products_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_products_category_id ON public.products USING btree (category_id);

--
-- Name: ix_products_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_products_id ON public.products USING btree (id);

--
-- Name: ix_products_material_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_products_material_id ON public.products USING btree (material_id);

--
-- Name: ix_products_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_products_name ON public.products USING btree (name);

--
-- Name: ix_products_sku; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ix_products_sku ON public.products USING btree (sku);

--
-- Name: price_history price_history_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.price_history
    ADD CONSTRAINT price_history_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE;

--
-- Name: products products_brand_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES public.brands(id);

--
-- Name: products products_category_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.categories(id);

--
-- Name: products products_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_material_id_fkey FOREIGN KEY (material_id) REFERENCES public.materials(id);

--
-- Name: products products_measurement_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_measurement_unit_id_fkey FOREIGN KEY (measurement_unit_id) REFERENCES public.measurement_units(id);
