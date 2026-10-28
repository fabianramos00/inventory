-- +goose Up
-- Brands, categories, materials, and measurement units.

--
-- Name: brands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.brands (
    id integer NOT NULL,
    name character varying(150) NOT NULL,
    logo_url character varying(255)
);

--
-- Name: brands_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.brands_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: brands_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.brands_id_seq OWNED BY public.brands.id;

--
-- Name: categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.categories (
    id integer NOT NULL,
    name character varying(150) NOT NULL,
    description character varying(500),
    code character varying(10)
);

--
-- Name: categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.categories_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.categories_id_seq OWNED BY public.categories.id;

--
-- Name: materials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.materials (
    id integer NOT NULL,
    name character varying(150) NOT NULL
);

--
-- Name: materials_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.materials_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: materials_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.materials_id_seq OWNED BY public.materials.id;

--
-- Name: measurement_units; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.measurement_units (
    id integer NOT NULL,
    name character varying(150) NOT NULL,
    abbreviation character varying(5) NOT NULL
);

--
-- Name: measurement_units_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.measurement_units_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: measurement_units_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.measurement_units_id_seq OWNED BY public.measurement_units.id;

--
-- Name: brands id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brands ALTER COLUMN id SET DEFAULT nextval('public.brands_id_seq'::regclass);

--
-- Name: categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categories ALTER COLUMN id SET DEFAULT nextval('public.categories_id_seq'::regclass);

--
-- Name: materials id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.materials ALTER COLUMN id SET DEFAULT nextval('public.materials_id_seq'::regclass);

--
-- Name: measurement_units id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.measurement_units ALTER COLUMN id SET DEFAULT nextval('public.measurement_units_id_seq'::regclass);

--
-- Name: brands brands_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT brands_name_key UNIQUE (name);

--
-- Name: brands brands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT brands_pkey PRIMARY KEY (id);

--
-- Name: categories categories_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categories
    ADD CONSTRAINT categories_name_key UNIQUE (name);

--
-- Name: categories categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.categories
    ADD CONSTRAINT categories_pkey PRIMARY KEY (id);

--
-- Name: materials materials_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.materials
    ADD CONSTRAINT materials_name_key UNIQUE (name);

--
-- Name: materials materials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.materials
    ADD CONSTRAINT materials_pkey PRIMARY KEY (id);

--
-- Name: measurement_units measurement_units_abbreviation_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.measurement_units
    ADD CONSTRAINT measurement_units_abbreviation_key UNIQUE (abbreviation);

--
-- Name: measurement_units measurement_units_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.measurement_units
    ADD CONSTRAINT measurement_units_name_key UNIQUE (name);

--
-- Name: measurement_units measurement_units_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.measurement_units
    ADD CONSTRAINT measurement_units_pkey PRIMARY KEY (id);

--
-- Name: ix_brands_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_brands_id ON public.brands USING btree (id);

--
-- Name: ix_categories_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ix_categories_code ON public.categories USING btree (code);

--
-- Name: ix_categories_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_categories_id ON public.categories USING btree (id);

--
-- Name: ix_materials_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_materials_id ON public.materials USING btree (id);

--
-- Name: ix_measurement_units_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_measurement_units_id ON public.measurement_units USING btree (id);
