# <insert more words>

# Snowflake Notes
- The user specified for `ORGADMIN` credentials must have the fields `NAME` and `LOGIN_NAME` match.
  To check this, you can use the following command in the Snowflake Console in a SQL worksheet:

    ```sql
    DESC USER <your user>; -- update the username here
    SELECT "property", "value"
        FROM TABLE(RESULT_SCAN(LAST_QUERY_ID()))
        WHERE "property" = 'NAME' OR "property" = 'LOGIN_NAME';
    ```

- This tool only supports RSA KeyPair authentication. Please see the [Snowflake Documentation][0]
  for more information about setting an RSA KeyPair for your `ORGADMIN` user.


[0]: https://docs.snowflake.com/en/user-guide/key-pair-auth#configuring-key-pair-authentication