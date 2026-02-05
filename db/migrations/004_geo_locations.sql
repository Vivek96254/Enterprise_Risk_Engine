-- Migration: Create geo_locations table with countries and cities
-- This table provides reference data for transaction location validation and risk assessment

-- Create geo_locations table
CREATE TABLE IF NOT EXISTS geo_locations (
    id SERIAL PRIMARY KEY,
    country_code VARCHAR(2) NOT NULL,
    country_name VARCHAR(100) NOT NULL,
    city_name VARCHAR(100),
    region VARCHAR(100),
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    timezone VARCHAR(50),
    risk_level VARCHAR(20) DEFAULT 'low', -- low, medium, high, sanctioned
    is_sanctioned BOOLEAN DEFAULT FALSE,
    population BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(country_code, city_name)
);

-- Create indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_geo_country_code ON geo_locations(country_code);
CREATE INDEX IF NOT EXISTS idx_geo_city_name ON geo_locations(city_name);
CREATE INDEX IF NOT EXISTS idx_geo_risk_level ON geo_locations(risk_level);
CREATE INDEX IF NOT EXISTS idx_geo_sanctioned ON geo_locations(is_sanctioned);

-- Insert countries with risk levels
INSERT INTO geo_locations (country_code, country_name, city_name, region, latitude, longitude, timezone, risk_level, is_sanctioned, population) VALUES
-- North America
('US', 'United States', 'New York', 'New York', 40.7128, -74.0060, 'America/New_York', 'low', false, 8336817),
('US', 'United States', 'Los Angeles', 'California', 34.0522, -118.2437, 'America/Los_Angeles', 'low', false, 3979576),
('US', 'United States', 'Chicago', 'Illinois', 41.8781, -87.6298, 'America/Chicago', 'low', false, 2693976),
('US', 'United States', 'Houston', 'Texas', 29.7604, -95.3698, 'America/Chicago', 'low', false, 2320268),
('US', 'United States', 'Phoenix', 'Arizona', 33.4484, -112.0740, 'America/Phoenix', 'low', false, 1680992),
('US', 'United States', 'San Francisco', 'California', 37.7749, -122.4194, 'America/Los_Angeles', 'low', false, 881549),
('US', 'United States', 'Seattle', 'Washington', 47.6062, -122.3321, 'America/Los_Angeles', 'low', false, 753675),
('US', 'United States', 'Miami', 'Florida', 25.7617, -80.1918, 'America/New_York', 'low', false, 467963),
('US', 'United States', 'Boston', 'Massachusetts', 42.3601, -71.0589, 'America/New_York', 'low', false, 692600),
('US', 'United States', 'Denver', 'Colorado', 39.7392, -104.9903, 'America/Denver', 'low', false, 727211),
('US', 'United States', 'Atlanta', 'Georgia', 33.7490, -84.3880, 'America/New_York', 'low', false, 498715),
('US', 'United States', 'Las Vegas', 'Nevada', 36.1699, -115.1398, 'America/Los_Angeles', 'medium', false, 641903),
('CA', 'Canada', 'Toronto', 'Ontario', 43.6532, -79.3832, 'America/Toronto', 'low', false, 2731571),
('CA', 'Canada', 'Vancouver', 'British Columbia', 49.2827, -123.1207, 'America/Vancouver', 'low', false, 631486),
('CA', 'Canada', 'Montreal', 'Quebec', 45.5017, -73.5673, 'America/Montreal', 'low', false, 1762949),
('CA', 'Canada', 'Calgary', 'Alberta', 51.0447, -114.0719, 'America/Edmonton', 'low', false, 1239220),
('MX', 'Mexico', 'Mexico City', 'CDMX', 19.4326, -99.1332, 'America/Mexico_City', 'medium', false, 21581000),
('MX', 'Mexico', 'Guadalajara', 'Jalisco', 20.6597, -103.3496, 'America/Mexico_City', 'medium', false, 1495182),
('MX', 'Mexico', 'Monterrey', 'Nuevo Leon', 25.6866, -100.3161, 'America/Monterrey', 'medium', false, 1142994),
('MX', 'Mexico', 'Cancun', 'Quintana Roo', 21.1619, -86.8515, 'America/Cancun', 'medium', false, 888797),

-- South America
('BR', 'Brazil', 'Sao Paulo', 'Sao Paulo', -23.5505, -46.6333, 'America/Sao_Paulo', 'medium', false, 12325232),
('BR', 'Brazil', 'Rio de Janeiro', 'Rio de Janeiro', -22.9068, -43.1729, 'America/Sao_Paulo', 'medium', false, 6747815),
('BR', 'Brazil', 'Brasilia', 'Distrito Federal', -15.7975, -47.8919, 'America/Sao_Paulo', 'low', false, 3055149),
('AR', 'Argentina', 'Buenos Aires', 'Buenos Aires', -34.6037, -58.3816, 'America/Argentina/Buenos_Aires', 'medium', false, 15153729),
('AR', 'Argentina', 'Cordoba', 'Cordoba', -31.4201, -64.1888, 'America/Argentina/Cordoba', 'medium', false, 1391000),
('CL', 'Chile', 'Santiago', 'Santiago', -33.4489, -70.6693, 'America/Santiago', 'low', false, 6158080),
('CO', 'Colombia', 'Bogota', 'Cundinamarca', 4.7110, -74.0721, 'America/Bogota', 'medium', false, 7181469),
('CO', 'Colombia', 'Medellin', 'Antioquia', 6.2476, -75.5658, 'America/Bogota', 'medium', false, 2569007),
('PE', 'Peru', 'Lima', 'Lima', -12.0464, -77.0428, 'America/Lima', 'medium', false, 10883000),
('VE', 'Venezuela', 'Caracas', 'Capital District', 10.4806, -66.9036, 'America/Caracas', 'high', false, 2082000),

-- Europe
('GB', 'United Kingdom', 'London', 'England', 51.5074, -0.1278, 'Europe/London', 'low', false, 8982000),
('GB', 'United Kingdom', 'Manchester', 'England', 53.4808, -2.2426, 'Europe/London', 'low', false, 553230),
('GB', 'United Kingdom', 'Birmingham', 'England', 52.4862, -1.8904, 'Europe/London', 'low', false, 1141816),
('GB', 'United Kingdom', 'Edinburgh', 'Scotland', 55.9533, -3.1883, 'Europe/London', 'low', false, 488050),
('DE', 'Germany', 'Berlin', 'Berlin', 52.5200, 13.4050, 'Europe/Berlin', 'low', false, 3644826),
('DE', 'Germany', 'Munich', 'Bavaria', 48.1351, 11.5820, 'Europe/Berlin', 'low', false, 1471508),
('DE', 'Germany', 'Frankfurt', 'Hesse', 50.1109, 8.6821, 'Europe/Berlin', 'low', false, 753056),
('DE', 'Germany', 'Hamburg', 'Hamburg', 53.5511, 9.9937, 'Europe/Berlin', 'low', false, 1841179),
('FR', 'France', 'Paris', 'Ile-de-France', 48.8566, 2.3522, 'Europe/Paris', 'low', false, 2161000),
('FR', 'France', 'Lyon', 'Auvergne-Rhone-Alpes', 45.7640, 4.8357, 'Europe/Paris', 'low', false, 513275),
('FR', 'France', 'Marseille', 'Provence-Alpes-Cote d''Azur', 43.2965, 5.3698, 'Europe/Paris', 'low', false, 861635),
('FR', 'France', 'Nice', 'Provence-Alpes-Cote d''Azur', 43.7102, 7.2620, 'Europe/Paris', 'low', false, 342522),
('IT', 'Italy', 'Rome', 'Lazio', 41.9028, 12.4964, 'Europe/Rome', 'low', false, 2872800),
('IT', 'Italy', 'Milan', 'Lombardy', 45.4642, 9.1900, 'Europe/Rome', 'low', false, 1352000),
('IT', 'Italy', 'Naples', 'Campania', 40.8518, 14.2681, 'Europe/Rome', 'medium', false, 959470),
('IT', 'Italy', 'Florence', 'Tuscany', 43.7696, 11.2558, 'Europe/Rome', 'low', false, 382258),
('ES', 'Spain', 'Madrid', 'Community of Madrid', 40.4168, -3.7038, 'Europe/Madrid', 'low', false, 3223334),
('ES', 'Spain', 'Barcelona', 'Catalonia', 41.3851, 2.1734, 'Europe/Madrid', 'low', false, 1620343),
('ES', 'Spain', 'Valencia', 'Valencia', 39.4699, -0.3763, 'Europe/Madrid', 'low', false, 791413),
('NL', 'Netherlands', 'Amsterdam', 'North Holland', 52.3676, 4.9041, 'Europe/Amsterdam', 'low', false, 872680),
('NL', 'Netherlands', 'Rotterdam', 'South Holland', 51.9244, 4.4777, 'Europe/Amsterdam', 'low', false, 651446),
('BE', 'Belgium', 'Brussels', 'Brussels', 50.8503, 4.3517, 'Europe/Brussels', 'low', false, 1208542),
('CH', 'Switzerland', 'Zurich', 'Zurich', 47.3769, 8.5417, 'Europe/Zurich', 'low', false, 402762),
('CH', 'Switzerland', 'Geneva', 'Geneva', 46.2044, 6.1432, 'Europe/Zurich', 'low', false, 201818),
('AT', 'Austria', 'Vienna', 'Vienna', 48.2082, 16.3738, 'Europe/Vienna', 'low', false, 1897491),
('SE', 'Sweden', 'Stockholm', 'Stockholm', 59.3293, 18.0686, 'Europe/Stockholm', 'low', false, 975904),
('NO', 'Norway', 'Oslo', 'Oslo', 59.9139, 10.7522, 'Europe/Oslo', 'low', false, 693494),
('DK', 'Denmark', 'Copenhagen', 'Capital Region', 55.6761, 12.5683, 'Europe/Copenhagen', 'low', false, 602481),
('FI', 'Finland', 'Helsinki', 'Uusimaa', 60.1699, 24.9384, 'Europe/Helsinki', 'low', false, 653835),
('PL', 'Poland', 'Warsaw', 'Masovia', 52.2297, 21.0122, 'Europe/Warsaw', 'low', false, 1790658),
('PL', 'Poland', 'Krakow', 'Lesser Poland', 50.0647, 19.9450, 'Europe/Warsaw', 'low', false, 779115),
('CZ', 'Czech Republic', 'Prague', 'Prague', 50.0755, 14.4378, 'Europe/Prague', 'low', false, 1309000),
('PT', 'Portugal', 'Lisbon', 'Lisbon', 38.7223, -9.1393, 'Europe/Lisbon', 'low', false, 504718),
('IE', 'Ireland', 'Dublin', 'Leinster', 53.3498, -6.2603, 'Europe/Dublin', 'low', false, 544107),
('GR', 'Greece', 'Athens', 'Attica', 37.9838, 23.7275, 'Europe/Athens', 'low', false, 664046),

-- Eastern Europe & Russia (Higher Risk)
('RU', 'Russia', 'Moscow', 'Moscow', 55.7558, 37.6173, 'Europe/Moscow', 'high', false, 12537954),
('RU', 'Russia', 'Saint Petersburg', 'Saint Petersburg', 59.9311, 30.3609, 'Europe/Moscow', 'high', false, 5383890),
('RU', 'Russia', 'Novosibirsk', 'Novosibirsk Oblast', 55.0084, 82.9357, 'Asia/Novosibirsk', 'high', false, 1612833),
('UA', 'Ukraine', 'Kyiv', 'Kyiv', 50.4501, 30.5234, 'Europe/Kiev', 'high', false, 2884000),
('UA', 'Ukraine', 'Odessa', 'Odessa Oblast', 46.4825, 30.7233, 'Europe/Kiev', 'high', false, 1015826),
('BY', 'Belarus', 'Minsk', 'Minsk', 53.9006, 27.5590, 'Europe/Minsk', 'high', false, 1982444),

-- Middle East
('AE', 'United Arab Emirates', 'Dubai', 'Dubai', 25.2048, 55.2708, 'Asia/Dubai', 'low', false, 3331420),
('AE', 'United Arab Emirates', 'Abu Dhabi', 'Abu Dhabi', 24.4539, 54.3773, 'Asia/Dubai', 'low', false, 1483000),
('SA', 'Saudi Arabia', 'Riyadh', 'Riyadh', 24.7136, 46.6753, 'Asia/Riyadh', 'medium', false, 7676654),
('SA', 'Saudi Arabia', 'Jeddah', 'Makkah', 21.4858, 39.1925, 'Asia/Riyadh', 'medium', false, 4076000),
('QA', 'Qatar', 'Doha', 'Doha', 25.2854, 51.5310, 'Asia/Qatar', 'low', false, 2382000),
('KW', 'Kuwait', 'Kuwait City', 'Al Asimah', 29.3759, 47.9774, 'Asia/Kuwait', 'medium', false, 2989000),
('BH', 'Bahrain', 'Manama', 'Capital', 26.2285, 50.5860, 'Asia/Bahrain', 'low', false, 411000),
('OM', 'Oman', 'Muscat', 'Muscat', 23.5880, 58.3829, 'Asia/Muscat', 'low', false, 1421409),
('IL', 'Israel', 'Tel Aviv', 'Tel Aviv', 32.0853, 34.7818, 'Asia/Jerusalem', 'medium', false, 460613),
('IL', 'Israel', 'Jerusalem', 'Jerusalem', 31.7683, 35.2137, 'Asia/Jerusalem', 'medium', false, 936425),
('TR', 'Turkey', 'Istanbul', 'Istanbul', 41.0082, 28.9784, 'Europe/Istanbul', 'medium', false, 15462452),
('TR', 'Turkey', 'Ankara', 'Ankara', 39.9334, 32.8597, 'Europe/Istanbul', 'medium', false, 5663322),
('EG', 'Egypt', 'Cairo', 'Cairo', 30.0444, 31.2357, 'Africa/Cairo', 'medium', false, 20076000),

-- Asia Pacific
('JP', 'Japan', 'Tokyo', 'Tokyo', 35.6762, 139.6503, 'Asia/Tokyo', 'low', false, 13960000),
('JP', 'Japan', 'Osaka', 'Osaka', 34.6937, 135.5023, 'Asia/Tokyo', 'low', false, 2752412),
('JP', 'Japan', 'Kyoto', 'Kyoto', 35.0116, 135.7681, 'Asia/Tokyo', 'low', false, 1475183),
('CN', 'China', 'Shanghai', 'Shanghai', 31.2304, 121.4737, 'Asia/Shanghai', 'medium', false, 24281400),
('CN', 'China', 'Beijing', 'Beijing', 39.9042, 116.4074, 'Asia/Shanghai', 'medium', false, 21542000),
('CN', 'China', 'Shenzhen', 'Guangdong', 22.5431, 114.0579, 'Asia/Shanghai', 'medium', false, 12528300),
('CN', 'China', 'Hong Kong', 'Hong Kong', 22.3193, 114.1694, 'Asia/Hong_Kong', 'low', false, 7500700),
('CN', 'China', 'Guangzhou', 'Guangdong', 23.1291, 113.2644, 'Asia/Shanghai', 'medium', false, 14904400),
('HK', 'Hong Kong', 'Hong Kong', 'Hong Kong', 22.3193, 114.1694, 'Asia/Hong_Kong', 'low', false, 7500700),
('SG', 'Singapore', 'Singapore', 'Singapore', 1.3521, 103.8198, 'Asia/Singapore', 'low', false, 5685807),
('KR', 'South Korea', 'Seoul', 'Seoul', 37.5665, 126.9780, 'Asia/Seoul', 'low', false, 9733509),
('KR', 'South Korea', 'Busan', 'Busan', 35.1796, 129.0756, 'Asia/Seoul', 'low', false, 3429000),
('TW', 'Taiwan', 'Taipei', 'Taipei', 25.0330, 121.5654, 'Asia/Taipei', 'low', false, 2646204),
('IN', 'India', 'Mumbai', 'Maharashtra', 19.0760, 72.8777, 'Asia/Kolkata', 'medium', false, 20411000),
('IN', 'India', 'Delhi', 'Delhi', 28.7041, 77.1025, 'Asia/Kolkata', 'medium', false, 16787941),
('IN', 'India', 'Bangalore', 'Karnataka', 12.9716, 77.5946, 'Asia/Kolkata', 'low', false, 8443675),
('IN', 'India', 'Chennai', 'Tamil Nadu', 13.0827, 80.2707, 'Asia/Kolkata', 'medium', false, 7088000),
('IN', 'India', 'Hyderabad', 'Telangana', 17.3850, 78.4867, 'Asia/Kolkata', 'low', false, 6993262),
('TH', 'Thailand', 'Bangkok', 'Bangkok', 13.7563, 100.5018, 'Asia/Bangkok', 'medium', false, 10539000),
('TH', 'Thailand', 'Phuket', 'Phuket', 7.8804, 98.3923, 'Asia/Bangkok', 'medium', false, 416582),
('VN', 'Vietnam', 'Ho Chi Minh City', 'Ho Chi Minh', 10.8231, 106.6297, 'Asia/Ho_Chi_Minh', 'medium', false, 8993082),
('VN', 'Vietnam', 'Hanoi', 'Hanoi', 21.0278, 105.8342, 'Asia/Ho_Chi_Minh', 'medium', false, 8053663),
('MY', 'Malaysia', 'Kuala Lumpur', 'Kuala Lumpur', 3.1390, 101.6869, 'Asia/Kuala_Lumpur', 'low', false, 1808000),
('ID', 'Indonesia', 'Jakarta', 'Jakarta', -6.2088, 106.8456, 'Asia/Jakarta', 'medium', false, 10562088),
('ID', 'Indonesia', 'Bali', 'Bali', -8.3405, 115.0920, 'Asia/Makassar', 'medium', false, 4225000),
('PH', 'Philippines', 'Manila', 'Metro Manila', 14.5995, 120.9842, 'Asia/Manila', 'medium', false, 1846513),
('AU', 'Australia', 'Sydney', 'New South Wales', -33.8688, 151.2093, 'Australia/Sydney', 'low', false, 5312163),
('AU', 'Australia', 'Melbourne', 'Victoria', -37.8136, 144.9631, 'Australia/Melbourne', 'low', false, 5078193),
('AU', 'Australia', 'Brisbane', 'Queensland', -27.4698, 153.0251, 'Australia/Brisbane', 'low', false, 2514184),
('AU', 'Australia', 'Perth', 'Western Australia', -31.9505, 115.8605, 'Australia/Perth', 'low', false, 2085973),
('NZ', 'New Zealand', 'Auckland', 'Auckland', -36.8509, 174.7645, 'Pacific/Auckland', 'low', false, 1657200),
('NZ', 'New Zealand', 'Wellington', 'Wellington', -41.2866, 174.7756, 'Pacific/Auckland', 'low', false, 215400),

-- Africa
('ZA', 'South Africa', 'Johannesburg', 'Gauteng', -26.2041, 28.0473, 'Africa/Johannesburg', 'medium', false, 5635127),
('ZA', 'South Africa', 'Cape Town', 'Western Cape', -33.9249, 18.4241, 'Africa/Johannesburg', 'medium', false, 4618000),
('NG', 'Nigeria', 'Lagos', 'Lagos', 6.5244, 3.3792, 'Africa/Lagos', 'high', false, 14862000),
('NG', 'Nigeria', 'Abuja', 'FCT', 9.0765, 7.3986, 'Africa/Lagos', 'high', false, 3464000),
('KE', 'Kenya', 'Nairobi', 'Nairobi', -1.2921, 36.8219, 'Africa/Nairobi', 'medium', false, 4397073),
('MA', 'Morocco', 'Casablanca', 'Casablanca-Settat', 33.5731, -7.5898, 'Africa/Casablanca', 'medium', false, 3359818),
('GH', 'Ghana', 'Accra', 'Greater Accra', 5.6037, -0.1870, 'Africa/Accra', 'medium', false, 2291352),

-- Sanctioned Countries (High Risk)
('IR', 'Iran', 'Tehran', 'Tehran', 35.6892, 51.3890, 'Asia/Tehran', 'sanctioned', true, 8693706),
('IR', 'Iran', 'Isfahan', 'Isfahan', 32.6546, 51.6680, 'Asia/Tehran', 'sanctioned', true, 1961260),
('KP', 'North Korea', 'Pyongyang', 'Pyongyang', 39.0392, 125.7625, 'Asia/Pyongyang', 'sanctioned', true, 3255388),
('SY', 'Syria', 'Damascus', 'Damascus', 33.5138, 36.2765, 'Asia/Damascus', 'sanctioned', true, 2079000),
('SY', 'Syria', 'Aleppo', 'Aleppo', 36.2021, 37.1343, 'Asia/Damascus', 'sanctioned', true, 1850000),
('CU', 'Cuba', 'Havana', 'Havana', 23.1136, -82.3666, 'America/Havana', 'sanctioned', true, 2130517),
('MM', 'Myanmar', 'Yangon', 'Yangon', 16.8661, 96.1951, 'Asia/Yangon', 'high', false, 5160512),

-- Country-only entries (for countries without specific cities)
('US', 'United States', NULL, NULL, 37.0902, -95.7129, 'America/New_York', 'low', false, 331002651),
('CA', 'Canada', NULL, NULL, 56.1304, -106.3468, 'America/Toronto', 'low', false, 37742154),
('GB', 'United Kingdom', NULL, NULL, 55.3781, -3.4360, 'Europe/London', 'low', false, 67886011),
('DE', 'Germany', NULL, NULL, 51.1657, 10.4515, 'Europe/Berlin', 'low', false, 83783942),
('FR', 'France', NULL, NULL, 46.2276, 2.2137, 'Europe/Paris', 'low', false, 65273511),
('JP', 'Japan', NULL, NULL, 36.2048, 138.2529, 'Asia/Tokyo', 'low', false, 126476461),
('AU', 'Australia', NULL, NULL, -25.2744, 133.7751, 'Australia/Sydney', 'low', false, 25499884),
('BR', 'Brazil', NULL, NULL, -14.2350, -51.9253, 'America/Sao_Paulo', 'medium', false, 212559417),
('IN', 'India', NULL, NULL, 20.5937, 78.9629, 'Asia/Kolkata', 'medium', false, 1380004385),
('CN', 'China', NULL, NULL, 35.8617, 104.1954, 'Asia/Shanghai', 'medium', false, 1439323776),
('RU', 'Russia', NULL, NULL, 61.5240, 105.3188, 'Europe/Moscow', 'high', false, 145934462),
('IR', 'Iran', NULL, NULL, 32.4279, 53.6880, 'Asia/Tehran', 'sanctioned', true, 83992949),
('KP', 'North Korea', NULL, NULL, 40.3399, 127.5101, 'Asia/Pyongyang', 'sanctioned', true, 25778816),
('SY', 'Syria', NULL, NULL, 34.8021, 38.9968, 'Asia/Damascus', 'sanctioned', true, 17500658),
('CU', 'Cuba', NULL, NULL, 21.5218, -77.7812, 'America/Havana', 'sanctioned', true, 11326616)
ON CONFLICT (country_code, city_name) DO UPDATE SET
    country_name = EXCLUDED.country_name,
    region = EXCLUDED.region,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    timezone = EXCLUDED.timezone,
    risk_level = EXCLUDED.risk_level,
    is_sanctioned = EXCLUDED.is_sanctioned,
    population = EXCLUDED.population,
    updated_at = NOW();

-- Create a view for easy querying of risky locations
CREATE OR REPLACE VIEW high_risk_locations AS
SELECT * FROM geo_locations 
WHERE risk_level IN ('high', 'sanctioned') OR is_sanctioned = true;

-- Create function to get location risk level
CREATE OR REPLACE FUNCTION get_location_risk(p_country_code VARCHAR(2), p_city_name VARCHAR(100) DEFAULT NULL)
RETURNS VARCHAR(20) AS $$
DECLARE
    v_risk_level VARCHAR(20);
BEGIN
    -- First try to find exact city match
    IF p_city_name IS NOT NULL THEN
        SELECT risk_level INTO v_risk_level
        FROM geo_locations
        WHERE country_code = p_country_code AND city_name = p_city_name
        LIMIT 1;
    END IF;
    
    -- If no city match, get country-level risk
    IF v_risk_level IS NULL THEN
        SELECT risk_level INTO v_risk_level
        FROM geo_locations
        WHERE country_code = p_country_code AND city_name IS NULL
        LIMIT 1;
    END IF;
    
    -- Default to medium if not found
    RETURN COALESCE(v_risk_level, 'medium');
END;
$$ LANGUAGE plpgsql;

-- Create function to check if location is sanctioned
CREATE OR REPLACE FUNCTION is_location_sanctioned(p_country_code VARCHAR(2))
RETURNS BOOLEAN AS $$
DECLARE
    v_sanctioned BOOLEAN;
BEGIN
    SELECT is_sanctioned INTO v_sanctioned
    FROM geo_locations
    WHERE country_code = p_country_code
    LIMIT 1;
    
    RETURN COALESCE(v_sanctioned, false);
END;
$$ LANGUAGE plpgsql;

-- Create function to calculate distance between two locations (Haversine formula)
CREATE OR REPLACE FUNCTION calculate_distance_km(
    lat1 DECIMAL, lon1 DECIMAL,
    lat2 DECIMAL, lon2 DECIMAL
) RETURNS DECIMAL AS $$
DECLARE
    R CONSTANT DECIMAL := 6371; -- Earth's radius in km
    dlat DECIMAL;
    dlon DECIMAL;
    a DECIMAL;
    c DECIMAL;
BEGIN
    dlat := radians(lat2 - lat1);
    dlon := radians(lon2 - lon1);
    a := sin(dlat/2) * sin(dlat/2) + cos(radians(lat1)) * cos(radians(lat2)) * sin(dlon/2) * sin(dlon/2);
    c := 2 * atan2(sqrt(a), sqrt(1-a));
    RETURN R * c;
END;
$$ LANGUAGE plpgsql;

-- Add comment
COMMENT ON TABLE geo_locations IS 'Reference table for geographic locations with risk levels for fraud detection';
