import unittest2

from mock import Mock

from book_file_processors import UnsupportedBookFileType
from file_importer import BookFileImporter
from settings import TESTS_DATA_ROOT


class FakeBookFileProcessorFactory(object):
    is_valid = None

    def __init__(self, *args, **kwargs):
        pass

    def get_book_file_processor(self):
        return FakeBookFileProcessor(self.is_valid)


class FakeBookFileProcessor(object):
    def __init__(self, is_valid):
        self.is_valid = is_valid

    def get_book(self):
        if not self.is_valid:
            raise UnsupportedBookFileType


class FakeDao(object):
    def __init__(self, file_was_imported):
        self.is_book_with_path_already_exists = Mock(return_value=file_was_imported)
        self.save_book = Mock()


class TestBookFileImporter(unittest2.TestCase):
    def test_file_is_saved_to_db_if_wasnt_earlier_imported(self):
        self.given_file_wasnt_imported()
        self.given_file_is_valid()
        self.when_try_to_import_file()
        self.then_book_is_saved_to_db()

    def test_nothing_happens_if_file_already_imported(self):
        self.given_file_already_was_imported()
        self.given_file_is_valid()
        self.when_try_to_import_file()
        self.then_nothing_is_saved_to_db()

    def test_nothing_happens_if_file_type_is_unsupported(self):
        self.given_file_wasnt_imported()
        self.given_file_is_NOT_valid()
        self.when_try_to_import_file()
        self.then_nothing_is_saved_to_db()

    # GIVEN

    def given_file_wasnt_imported(self):
        self.file_was_imported = False

    def given_file_already_was_imported(self):
        self.file_was_imported = True

    def given_file_is_valid(self):
        self.file_is_valid = True

    def given_file_is_NOT_valid(self):
        self.file_is_valid = False

    # WHEN

    def when_try_to_import_file(self):
        self.importer = self.get_book_file_importer()
        self.importer.try_to_import_file_if_not_already_imported(TESTS_DATA_ROOT)

    def get_book_file_importer(self):
        importer = BookFileImporter()
        FakeBookFileProcessorFactory.is_valid = self.file_is_valid
        importer.book_processor_factory_cls = FakeBookFileProcessorFactory
        importer.dao = FakeDao(file_was_imported=self.file_was_imported)
        return importer



    # THEN

    def then_book_is_saved_to_db(self):
        self.assertTrue(self.importer.dao.save_book.called)

    def then_nothing_is_saved_to_db(self):
       self.assertFalse(self.importer.dao.save_book.called)
