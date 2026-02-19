# file-transfer

It might work.

## Cross-compatibility changes

I was able to replace my proto file with my teamate's and it worked without having to make any changes.

The GET protocol was changed. Originally, my implementation sent the checksum bundled together with the retrieval response (size + checksum in one message), while my partner's sent size only in the response and then sent the checksum as a separate message after streaming the file.

PUT was already compatible between both implementations with no changes needed.