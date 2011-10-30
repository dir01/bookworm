from book_file_processors import UnsupportedBookFileType
from directory_importer import BookFileImporter

class FakeBookFileProcessorFactory(object):
    def __init__(self, path):
        pass

    def get_book_file_processor(self):
        return FakeBookFileProcessor()


class FakeBookFileProcessor(object):
    file_is_valid = True
    valid_fake_book = 'Really anything. The guy is dumb'

    def get_book(self):
        if self.file_is_valid:
            return self.valid_fake_book
        else:
            raise UnsupportedBookFileType

    @classmethod
    def reset(cls):
        cls.file_is_valid = True



class FakeDb(object):
    already_imported = None
    book = None

    @classmethod
    def book_with_path_already_exists(cls, path):
        return cls.already_imported

    @classmethod
    def save_book(cls, book):
        cls.book = book

    @classmethod
    def reset(cls):
        cls.already_imported = None
        cls.book = None


class TestBookFileImporter(object):
    def setup(self):
        FakeDb.reset()
        FakeBookFileProcessor.reset()

    def get_book_file_importer(self):
        importer = BookFileImporter(path=None)
        importer.BookFileProcessorFactory = FakeBookFileProcessorFactory
        importer.db = FakeDb
        return importer

    def test_file_is_saved_to_db_if_wasnt_earlier_imported(self):
        self.given_file_wasnt_imported()
        self.when_try_to_import_file()
        self.then_book_is_saved_to_db()

    def test_nothing_happens_if_file_already_imported(self):
        self.given_file_already_was_imported()
        self.when_try_to_import_file()
        self.then_nothing_is_saved_to_db()

    def test_nothing_happens_if_file_type_is_unsupported(self):
        self.given_file_wasnt_imported()
        self.when_try_to_import_wrong_file()
        self.then_nothing_is_saved_to_db()

    # GIVEN

    def given_file_wasnt_imported(self):
        FakeDb.already_imported = False

    def given_file_already_was_imported(self):
        FakeDb.already_imported = True

    # WHEN

    def when_try_to_import_file(self):
        self.try_to_import_file()

    def when_try_to_import_wrong_file(self):
        FakeBookFileProcessor.file_is_valid = False
        self.try_to_import_file()

    def try_to_import_file(self):
        self.get_book_file_importer().try_to_import_file_if_not_already_imported()

    # THEN

    def then_book_is_saved_to_db(self):
        assert FakeDb.book

    def then_nothing_is_saved_to_db(self):
       assert not FakeDb.book
