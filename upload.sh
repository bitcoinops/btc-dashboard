#!/bin/bash
# Uploads json file to amazon S3 bucket

while true; do
    # Avoid touching the most recent file as an easy away to avoid
    # a data race with program writing to it.
    mostRecentBlock=$(ls $1 | cut -f 1 -d '.' | sort --n | tail -1)
    j=".json"
    fileName=$mostRecentBlock$j

    # Move all files except for the one from the most recent block
    # in the temp_dir
    find $1 -maxdepth 1 -mindepth 1 -not -name $fileName -print0 |
        xargs -0 mv -t temp_dir

    # Upload the individual JSON files
    aws s3 cp --recursive temp_dir/ s3://dashboard.dataset/blocks

    # Update existing archive with new files
    cd temp_dir
    tar -uf ../bitcoinops-dataset.tar .
    cd ..

    # Zip 
    gzip bitcoinops-dataset.tar

    # Upload to S3
    aws s3 cp ./bitcoinops-dataset.tar.gz s3://dashboard.dataset/backups/bitcoinops-dataset.tar.gz --metadata HEIGHT=$mostRecentBlock

    gunzip bitcoinops-dataset.tar

    # Move JSON files to main directory.
    mv ./temp_dir/* $2

    sleep 6h
done
