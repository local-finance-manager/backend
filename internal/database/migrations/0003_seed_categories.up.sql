-- Canonical system seed: 24 categories, 142 subcategories (can_be_deleted = 0)

-- ─── Categorias de Despesa ───────────────────────────────────────────────────

INSERT OR IGNORE INTO categories (id, name, type, icon, color, can_be_deleted, created_at, updated_at) VALUES
('cat-moradia',        'Moradia',                    'despesa', NULL, '#4A90D9', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-alimentacao',    'Alimentação',                'despesa', NULL, '#F5A623', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-transporte',     'Transporte',                 'despesa', NULL, '#7B68EE', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-saude',          'Saúde',                      'despesa', NULL, '#E74C3C', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-educacao',       'Educação',                   'despesa', NULL, '#2ECC71', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-vestuario',      'Vestuário',                  'despesa', NULL, '#E91E63', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-lazer',          'Entretenimento e Lazer',     'despesa', NULL, '#FF9800', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-pets',           'Pets',                       'despesa', NULL, '#795548', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-cartao',         'Cartão de Crédito',          'despesa', NULL, '#607D8B', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-impostos',       'Impostos e Taxas',           'despesa', NULL, '#9C27B0', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-seguros',        'Seguros',                    'despesa', NULL, '#00BCD4', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-beleza',         'Beleza e Cuidados Pessoais', 'despesa', NULL, '#FF4081', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-tecnologia',     'Tecnologia',                 'despesa', NULL, '#3F51B5', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-doacoes',        'Presentes e Doações',        'despesa', NULL, '#009688', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-filhos',         'Filhos',                     'despesa', NULL, '#CDDC39', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-domestico',      'Doméstico e Limpeza',        'despesa', NULL, '#8BC34A', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-viagens',        'Viagens',                    'despesa', NULL, '#FF5722', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-outros-gastos',  'Outros Gastos',              'despesa', NULL, '#9E9E9E', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Categorias de Receita ────────────────────────────────────────────────────

INSERT OR IGNORE INTO categories (id, name, type, icon, color, can_be_deleted, created_at, updated_at) VALUES
('cat-salario',            'Salário e Rendimentos do Trabalho', 'receita', NULL, '#27AE60', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-investimentos',      'Renda de Investimentos',            'receita', NULL, '#F39C12', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-renda-extra',        'Renda Extra',                       'receita', NULL, '#1ABC9C', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-beneficios',         'Benefícios',                        'receita', NULL, '#3498DB', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('cat-outros-rendimentos', 'Outros Rendimentos',                'receita', NULL, '#BDC3C7', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Categoria de Transferência ───────────────────────────────────────────────

INSERT OR IGNORE INTO categories (id, name, type, icon, color, can_be_deleted, created_at, updated_at) VALUES
('cat-transferencias', 'Transferências', 'transferencia', NULL, '#546E7A', 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Moradia (10) ──────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-mor-aluguel',    'cat-moradia', 'Aluguel',                 NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-condominio', 'cat-moradia', 'Condomínio',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-agua',       'cat-moradia', 'Água e Esgoto',           NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-energia',    'cat-moradia', 'Energia Elétrica',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-gas',        'cat-moradia', 'Gás',                     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-internet',   'cat-moradia', 'Internet e Telefone Fixo',NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-iptu',       'cat-moradia', 'IPTU',                    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-manutencao', 'cat-moradia', 'Manutenção e Reparos',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-decoracao',  'cat-moradia', 'Decoração e Mobília',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-mor-seguranca',  'cat-moradia', 'Vigilância e Segurança',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Alimentação (8) ──────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-ali-supermercado', 'cat-alimentacao', 'Supermercado',               NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-restaurante',  'cat-alimentacao', 'Restaurante',                NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-delivery',     'cat-alimentacao', 'Delivery',                   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-padaria',      'cat-alimentacao', 'Padaria e Cafeteria',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-feira',        'cat-alimentacao', 'Feira e Sacolão',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-bebidas',      'cat-alimentacao', 'Bebidas e Bar',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-mercearia',    'cat-alimentacao', 'Mercearia e Prod. Naturais', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ali-fast-food',    'cat-alimentacao', 'Fast Food e Lanchonete',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Transporte (9) ───────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-tra-combustivel', 'cat-transporte', 'Combustível',                   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-pedagio',     'cat-transporte', 'Pedágio e Estacionamento',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-manutencao',  'cat-transporte', 'Manutenção do Veículo',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-publico',     'cat-transporte', 'Transporte Público',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-aplicativo',  'cat-transporte', 'Aplicativo de Transporte',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-seguro',      'cat-transporte', 'Seguro e DPVAT',                NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-ipva',        'cat-transporte', 'IPVA e Licenciamento',          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-taxi',        'cat-transporte', 'Táxi',                          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tra-bicicleta',   'cat-transporte', 'Bicicleta e Patinete Elétrico', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Saúde (8) ────────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-sau-plano',       'cat-saude', 'Plano de Saúde',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-consulta',    'cat-saude', 'Consultas Médicas',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-exame',       'cat-saude', 'Exames e Laboratórios',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-medicamento', 'cat-saude', 'Medicamentos e Farmácia',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-dentista',    'cat-saude', 'Dentista',                  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-psicologo',   'cat-saude', 'Psicólogo e Terapia',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-academia',    'cat-saude', 'Academia e Atividade Física',NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sau-otica',       'cat-saude', 'Óculos e Ótica',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Educação (7) ─────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-edu-mensalidade', 'cat-educacao', 'Mensalidade Escola / Faculdade', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-curso',       'cat-educacao', 'Cursos e Capacitação',           NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-livros',      'cat-educacao', 'Livros e Material Didático',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-idioma',      'cat-educacao', 'Idiomas',                         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-material',    'cat-educacao', 'Material Escolar',                NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-evento',      'cat-educacao', 'Eventos e Congressos',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-edu-papelaria',   'cat-educacao', 'Papelaria',                       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Vestuário (5) ────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-ves-roupas',     'cat-vestuario', 'Roupas',                     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ves-calcados',   'cat-vestuario', 'Calçados',                   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ves-acessorios', 'cat-vestuario', 'Acessórios',                 NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ves-intima',     'cat-vestuario', 'Moda Íntima e Lingerie',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ves-trabalho',   'cat-vestuario', 'Roupas e Uniformes de Trabalho', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Entretenimento e Lazer (8) ───────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-laz-streaming', 'cat-lazer', 'Streaming (Netflix, Spotify...)', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-cinema',    'cat-lazer', 'Cinema e Teatro',                 NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-shows',     'cat-lazer', 'Shows e Eventos',                 NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-jogos',     'cat-lazer', 'Jogos e Videogames',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-hobby',     'cat-lazer', 'Hobbies e Colecionismo',          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-clube',     'cat-lazer', 'Clube e Associações',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-parques',   'cat-lazer', 'Parques e Atrações',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-laz-festas',    'cat-lazer', 'Festas e Comemorações',           NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Pets (6) ─────────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-pet-racao',        'cat-pets', 'Ração e Petiscos',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-pet-veterinario',  'cat-pets', 'Veterinário e Clínica',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-pet-vacinas',      'cat-pets', 'Vacinas e Exames',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-pet-petshop',      'cat-pets', 'Pet Shop e Acessórios',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-pet-remedios',     'cat-pets', 'Medicamentos Veterinários',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-pet-adestramento', 'cat-pets', 'Adestramento e Cuidados',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Cartão de Crédito (5) ────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-car-visa',    'cat-cartao', 'Fatura Visa',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-car-master',  'cat-cartao', 'Fatura Mastercard',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-car-elo',     'cat-cartao', 'Fatura Elo',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-car-amex',    'cat-cartao', 'Fatura American Express', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-car-outros',  'cat-cartao', 'Fatura Outros Cartões',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Impostos e Taxas (6) ─────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-imp-irpf',    'cat-impostos', 'Imposto de Renda (IRPF)',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-imp-iof',     'cat-impostos', 'IOF',                      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-imp-tarifas', 'cat-impostos', 'Tarifas Bancárias',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-imp-cartorio','cat-impostos', 'Taxas Cartoriais',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-imp-multas',  'cat-impostos', 'Multas e Infrações',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-imp-itr',     'cat-impostos', 'ITR e Outros Impostos',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Seguros (5) ──────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-seg-vida',         'cat-seguros', 'Seguro de Vida',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-seg-residencial',  'cat-seguros', 'Seguro Residencial',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-seg-viagem',       'cat-seguros', 'Seguro Viagem',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-seg-equipamentos', 'cat-seguros', 'Seguro Equipamentos',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-seg-outros',       'cat-seguros', 'Outros Seguros',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Beleza e Cuidados Pessoais (5) ───────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-bel-cabelo',    'cat-beleza', 'Cabelereiro e Salão',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-bel-estetica',  'cat-beleza', 'Estética e Spa',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-bel-manicure',  'cat-beleza', 'Manicure e Pedicure',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-bel-cosmeticos','cat-beleza', 'Cosméticos e Perfumaria',NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-bel-higiene',   'cat-beleza', 'Higiene Pessoal',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Tecnologia (6) ───────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-tec-eletronicos',  'cat-tecnologia', 'Eletrônicos e Gadgets',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tec-software',     'cat-tecnologia', 'Software e Aplicativos',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tec-assinaturas',  'cat-tecnologia', 'Assinaturas Digitais',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tec-manutencao',   'cat-tecnologia', 'Manutenção de Equipamentos',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tec-acessorios',   'cat-tecnologia', 'Acessórios de Informática',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-tec-celular',      'cat-tecnologia', 'Celular e Plano Móvel',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Presentes e Doações (4) ──────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-doa-presentes',  'cat-doacoes', 'Presentes',                  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-doa-doacoes',    'cat-doacoes', 'Doações e Contribuições',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-doa-mesadas',    'cat-doacoes', 'Mesada e Ajuda Familiar',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-doa-religioso',  'cat-doacoes', 'Religioso e Dízimo',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Filhos (6) ───────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-fil-escola',    'cat-filhos', 'Escola e Extracurriculares',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-fil-brinquedos','cat-filhos', 'Brinquedos e Diversão',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-fil-roupas',    'cat-filhos', 'Roupas Infantis',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-fil-baba',      'cat-filhos', 'Babá e Cuidadores',           NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-fil-saude',     'cat-filhos', 'Saúde Infantil',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-fil-material',  'cat-filhos', 'Materiais Escolares Infantis',NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Doméstico e Limpeza (5) ──────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-dom-empregada',  'cat-domestico', 'Empregada Doméstica',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-dom-limpeza',    'cat-domestico', 'Produtos de Limpeza',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-dom-lavanderia', 'cat-domestico', 'Lavanderia',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-dom-utensilios', 'cat-domestico', 'Utensílios Domésticos',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-dom-jardinagem', 'cat-domestico', 'Jardinagem',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Viagens (6) ──────────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-via-passagem',    'cat-viagens', 'Passagens Aéreas e Terrestres', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-via-hotel',       'cat-viagens', 'Hospedagem e Hotel',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-via-passeios',    'cat-viagens', 'Passeios e Tours',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-via-alimentacao', 'cat-viagens', 'Alimentação na Viagem',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-via-seguro',      'cat-viagens', 'Seguro Viagem',                 NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-via-outros',      'cat-viagens', 'Outros Gastos de Viagem',       NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Outros Gastos (3) ────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-outg-geral',           'cat-outros-gastos', 'Gastos Gerais',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-outg-emergencia',      'cat-outros-gastos', 'Emergências',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-outg-nao-classificado','cat-outros-gastos', 'Não Classificado', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Salário e Rendimentos (6) ────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-sal-salario',          'cat-salario', 'Salário Mensal',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sal-decimo-terceiro',  'cat-salario', '13º Salário',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sal-ferias',           'cat-salario', 'Férias',              NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sal-bonus',            'cat-salario', 'Bônus e Comissões',   NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sal-horas-extras',     'cat-salario', 'Horas Extras',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-sal-pro-labore',       'cat-salario', 'Pró-Labore',          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Renda de Investimentos (6) ───────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-inv-dividendos', 'cat-investimentos', 'Dividendos e JCP',          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-inv-cdb',        'cat-investimentos', 'CDB e Renda Fixa',          NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-inv-tesouro',    'cat-investimentos', 'Tesouro Direto',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-inv-fundos',     'cat-investimentos', 'Fundos de Investimento',     NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-inv-acoes',      'cat-investimentos', 'Ações e ETFs',               NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-inv-imoveis',    'cat-investimentos', 'Renda de Imóveis (FIIs)',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Renda Extra (5) ──────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-rex-freelance', 'cat-renda-extra', 'Freelance e Consultoria',        NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-rex-vendas',    'cat-renda-extra', 'Vendas de Produtos / Serviços',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-rex-aluguel',   'cat-renda-extra', 'Aluguel Recebido',               NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-rex-cashback',  'cat-renda-extra', 'Cashback e Reembolso',           NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-rex-premios',   'cat-renda-extra', 'Prêmios e Sorteios',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Benefícios (4) ───────────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-ben-vale-alimentacao', 'cat-beneficios', 'Vale Alimentação',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ben-vale-transporte',  'cat-beneficios', 'Vale Transporte',             NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ben-auxilio-saude',    'cat-beneficios', 'Auxílio Saúde',               NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-ben-plr',              'cat-beneficios', 'PLR e Participação nos Resultados', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Outros Rendimentos (4) ───────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-outr-heranca',          'cat-outros-rendimentos', 'Herança e Doações Recebidas', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-outr-restituicao',      'cat-outros-rendimentos', 'Restituição de IRPF',         NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-outr-indenizacoes',     'cat-outros-rendimentos', 'Indenizações Recebidas',      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-outr-nao-classificado', 'cat-outros-rendimentos', 'Não Classificado',            NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');

-- ─── Subcategorias: Transferências (5) ───────────────────────────────────────

INSERT OR IGNORE INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at) VALUES
('sub-trf-pix',          'cat-transferencias', 'PIX',                      NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-ted',          'cat-transferencias', 'TED e DOC',                NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-entre-contas', 'cat-transferencias', 'Entre Contas Próprias',    NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-aporte',       'cat-transferencias', 'Aporte em Investimentos',  NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'),
('sub-trf-resgate',      'cat-transferencias', 'Resgate de Investimentos', NULL, NULL, 0, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z');
