import os
from xdg import BaseDirectory

PROGRAM_NAME = 'bookworm'

PROJECT_ROOT = os.path.dirname(__file__)
TESTS_ROOT = os.path.join(PROJECT_ROOT, 'tests')
TESTS_DATA_ROOT = os.path.join(TESTS_ROOT, 'data')

SQLITE_DB_ROOT = os.path.join(BaseDirectory.xdg_data_home, PROGRAM_NAME)
if not os.path.exists(SQLITE_DB_ROOT):
    os.makedirs(SQLITE_DB_ROOT)
SQLALCHEMY_URL = 'sqlite:///%s/db.sqlite' % SQLITE_DB_ROOT