-- 007_genres_pt: the genres only series have on TMDB come joined and in English
-- even in pt-BR ("Sci-Fi & Fantasy"). They become the movie genres, so a row has
-- movies and series together (same table as internal/metadata/client.go).

INSERT OR IGNORE INTO genres (id, name)
SELECT v.id, v.name
FROM (
    SELECT 10759 AS old, 28 AS id, 'Ação' AS name
    UNION ALL SELECT 10759, 12, 'Aventura'
    UNION ALL SELECT 10765, 878, 'Ficção científica'
    UNION ALL SELECT 10765, 14, 'Fantasia'
    UNION ALL SELECT 10768, 10752, 'Guerra'
) v
WHERE v.old IN (SELECT genre_id FROM title_genres);

INSERT OR IGNORE INTO title_genres (owner_id, genre_id)
SELECT tg.owner_id, v.id
FROM title_genres tg
JOIN (
    SELECT 10759 AS old, 28 AS id
    UNION ALL SELECT 10759, 12
    UNION ALL SELECT 10765, 878
    UNION ALL SELECT 10765, 14
    UNION ALL SELECT 10768, 10752
) v ON v.old = tg.genre_id;

DELETE FROM title_genres WHERE genre_id IN (10759, 10765);
DELETE FROM genres WHERE id IN (10759, 10765);

UPDATE genres SET name = 'Política' WHERE id = 10768;
UPDATE genres SET name = 'Infantil' WHERE id = 10762;
UPDATE genres SET name = 'Notícias' WHERE id = 10763;
UPDATE genres SET name = 'Novela' WHERE id = 10766;
UPDATE genres SET name = 'Programa de entrevistas' WHERE id = 10767;
