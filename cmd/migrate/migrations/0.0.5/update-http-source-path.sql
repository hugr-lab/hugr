
UPDATE data_sources 
    SET path = replace(path, 'x-jacke', 'x-hugr') 
WHERE type = 'http' and contains(path, 'x-jacke');